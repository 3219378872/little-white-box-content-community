package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/user/rpc/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

const maxUnreadWatchHits = 5

// Runtime 把 Intent / Policy / Planner 叠在既有 Runner（Executor）之上。
// 当前执行器仍是 OpenAIRunner；新工具分组落地后只改 Policy 与工具表。
type Runtime struct {
	Executor Runner
	Memory   memory.Store
	Watch    watch.Store
	Audit    AuditStore
	User     userservice.UserService
	Content  contentservice.ContentService
	Model    string
}

func NewRuntime(executor Runner, store memory.Store) *Runtime {
	if executor == nil {
		return nil
	}
	return &Runtime{Executor: executor, Memory: store}
}

func (r *Runtime) Run(ctx context.Context, session *Session) (*Result, error) {
	if r == nil || r.Executor == nil {
		return nil, ErrLLMUnavailable
	}
	if session == nil {
		return nil, ErrLLMUnavailable
	}
	started := time.Now()
	session.Plan = ClassifyIntent(session.UserMessage)
	session.Tools = RestrictToolsForConsent(session.Tools, session.ConsentVersion)
	skipBehavior := r.skipBehaviorSources(ctx, session.UserID)
	if r.Memory != nil && session.UserID > 0 {
		if block, err := r.Memory.ContextBlock(ctx, session.UserID, session.Plan.Intent, time.Now(), skipBehavior); err != nil {
			logx.WithContext(ctx).Infow("agent memory context skipped", logx.Field("err", err.Error()))
		} else if block != "" {
			session.MemoryContext = block
		}
	}
	if r.Watch != nil && session.UserID > 0 {
		if hits, err := r.Watch.ListHits(ctx, session.UserID, true); err != nil {
			logx.WithContext(ctx).Infow("agent watch hits skipped", logx.Field("err", err.Error()))
		} else if block := formatUnreadWatchHits(r.filterVisibleWatchHits(ctx, hits)); block != "" {
			session.WatchContext = block
		}
	}
	logx.WithContext(ctx).Infow("agent runtime planned",
		logx.Field("intent", session.Plan.Intent),
		logx.Field("consentVersion", session.ConsentVersion),
		logx.Field("requestId", session.RequestID),
	)
	result, err := r.Executor.Run(ctx, session)
	r.persistAudit(session, time.Since(started), err)
	if err == nil && r.Memory != nil && session.UserID > 0 {
		r.persistMemory(session)
	}
	return result, err
}

func (r *Runtime) skipBehaviorSources(ctx context.Context, userID int64) bool {
	if r == nil || r.User == nil || userID <= 0 {
		return true
	}
	pref, err := r.User.GetPersonalizationPreference(ctx, &userservice.GetPersonalizationPreferenceReq{UserId: userID})
	if err != nil || pref == nil {
		return true
	}
	return !pref.Enabled
}

func (r *Runtime) filterVisibleWatchHits(ctx context.Context, hits []watch.Hit) []watch.Hit {
	out := append([]watch.Hit(nil), hits...)
	ids := make([]int64, 0, len(out))
	for _, hit := range out {
		if hit.PostID > 0 {
			ids = append(ids, hit.PostID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	if r == nil || r.Content == nil {
		return redactWatchHitContent(out)
	}
	published, err := tool.PublishedPosts(ctx, r.Content, ids)
	if err != nil {
		logx.WithContext(ctx).Infow("agent watch visibility check failed", logx.Field("err", err.Error()))
		return redactWatchHitContent(out)
	}
	for i := range out {
		if out[i].PostID <= 0 {
			continue
		}
		info := published[out[i].PostID]
		if info == nil {
			out[i].Title = ""
			out[i].Summary = ""
			continue
		}
		out[i].Title = info.Title
	}
	return out
}

func redactWatchHitContent(hits []watch.Hit) []watch.Hit {
	for i := range hits {
		if hits[i].PostID > 0 {
			hits[i].Title = ""
			hits[i].Summary = ""
		}
	}
	return hits
}

func formatUnreadWatchHits(hits []watch.Hit) string {
	if len(hits) == 0 {
		return ""
	}
	sorted := append([]watch.Hit(nil), hits...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt == sorted[j].CreatedAt {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})
	var b strings.Builder
	b.WriteString("未读的条件追踪命中（仅供参考，不能替代帖子证据）：\n")
	n := 0
	for _, hit := range sorted {
		if hit.Read {
			continue
		}
		n++
		if n > maxUnreadWatchHits {
			break
		}
		summary := strings.TrimSpace(hit.Summary)
		title := strings.TrimSpace(hit.Title)
		fmt.Fprintf(&b, "- hit=%d post=%d %s %s\n", hit.ID, hit.PostID, title, summary)
	}
	if n == 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *Runtime) persistAudit(session *Session, latency time.Duration, runErr error) {
	if r == nil || r.Audit == nil || session == nil {
		return
	}
	status := "ok"
	if runErr != nil {
		status = "error"
	}
	rec := RunRecord{
		UserID:         session.UserID,
		RequestID:      session.RequestID,
		ConversationID: session.ConversationID,
		Intent:         session.Plan.Intent,
		Model:          r.Model,
		LatencyMS:      int(latency.Milliseconds()),
		Status:         status,
		Tools:          append([]ToolAudit(nil), session.toolAudits...),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.Audit.Record(ctx, rec); err != nil {
			logx.Errorw("agent audit persist failed", logx.Field("err", err.Error()))
		}
	}()
}

func (r *Runtime) persistMemory(session *Session) {
	candidates := memory.Extract(session.UserMessage)
	if len(candidates) == 0 {
		return
	}
	store := r.Memory
	userID := session.UserID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		now := time.Now()
		for _, candidate := range candidates {
			if err := store.Apply(ctx, userID, candidate, now); err != nil {
				logx.Errorw("agent memory apply failed", logx.Field("err", err.Error()))
			}
		}
	}()
}
