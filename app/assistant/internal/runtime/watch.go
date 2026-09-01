package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/watch"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	watchWindow      = 2 * time.Minute
	watchDailyLimit  = 20
	watchHourlyLimit = 3
	watchHitsMarker  = "UNTRUSTED_WATCH_HITS_JSON"
)

type ConsentChecker func(ctx context.Context, userID int64) (bool, error)
type WatchPostVisibility func(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)

var errNoVisibleWatchHits = errors.New("watch bucket has no currently visible posts")

type watchRunPayload struct {
	BucketID int64   `json:"bucket_id"`
	HitIDs   []int64 `json:"hit_ids"`
	TaskIDs  []int64 `json:"task_ids,omitempty"`
	PostIDs  []int64 `json:"post_ids,omitempty"`
}

func ScheduleDueWatchRuns(ctx context.Context, st store.Store, memories memory.Store, watchStore watch.Store, consent ConsentChecker, visible WatchPostVisibility) {
	if st == nil {
		return
	}
	if err := st.RequeueFailedBuckets(ctx); err != nil {
		logx.WithContext(ctx).Errorw("requeue failed watch buckets", logx.Field("err", err.Error()))
		return
	}
	now := store.NowMs()
	buckets, err := st.ListDueBuckets(ctx, now, watchWindow.Milliseconds())
	if err != nil || len(buckets) == 0 {
		return
	}
	for _, bucket := range buckets {
		if err := scheduleBucket(ctx, st, memories, watchStore, consent, visible, bucket, now); err != nil {
			logx.WithContext(ctx).Errorw("schedule watch run failed", logx.Field("bucket", bucket.ID), logx.Field("err", err.Error()))
		}
	}
}

func scheduleBucket(ctx context.Context, st store.Store, memories memory.Store, watchStore watch.Store, consent ConsentChecker, visible WatchPostVisibility, bucket store.DeliveryBucket, now int64) error {
	consentVersion, granted, err := st.AgentConsent(ctx, bucket.UserID)
	if err != nil {
		return err
	}
	if !granted || consentVersion <= 0 {
		return st.DeferBucket(ctx, bucket.ID, now+watchWindow.Milliseconds())
	}
	if consent != nil {
		externalGranted, consentErr := consent(ctx, bucket.UserID)
		if consentErr != nil {
			return consentErr
		}
		if !externalGranted {
			return st.DeferBucket(ctx, bucket.ID, now+watchWindow.Milliseconds())
		}
	}
	if watchStore == nil {
		return fmt.Errorf("watch bucket %d has no hit store", bucket.ID)
	}
	if len(bucket.HitIDs) == 0 {
		return st.DismissBucket(ctx, bucket.ID, 0)
	}
	taskIDs := make([]int64, 0)
	postIDs := make([]int64, 0)
	visibleByID := map[int64]watch.Hit{}
	if watchStore != nil && len(bucket.HitIDs) > 0 {
		hits, listErr := watchStore.GetHitsByIDs(ctx, bucket.UserID, bucket.HitIDs)
		if listErr != nil {
			return listErr
		}
		if len(hits) == 0 {
			return st.DismissBucket(ctx, bucket.ID, 0)
		}
		hits, listErr = currentlyVisibleWatchHits(ctx, bucket.UserID, hits, visible)
		if listErr != nil {
			return listErr
		}
		if len(hits) == 0 {
			return st.DismissBucket(ctx, bucket.ID, 0)
		}
		for _, hit := range hits {
			visibleByID[hit.ID] = hit
		}
		seen := map[int64]struct{}{}
		seenPosts := map[int64]struct{}{}
		for _, id := range bucket.HitIDs {
			hit, ok := visibleByID[id]
			if !ok {
				continue
			}
			if _, dup := seen[hit.TaskID]; dup {
				continue
			}
			seen[hit.TaskID] = struct{}{}
			taskIDs = append(taskIDs, hit.TaskID)
			if hit.PostID > 0 {
				if _, exists := seenPosts[hit.PostID]; !exists {
					seenPosts[hit.PostID] = struct{}{}
					postIDs = append(postIDs, hit.PostID)
				}
			}
		}
	}
	if len(taskIDs) == 0 {
		return st.DismissBucket(ctx, bucket.ID, 0)
	}
	sort.Slice(taskIDs, func(i, j int) bool { return taskIDs[i] < taskIDs[j] })
	sort.Slice(postIDs, func(i, j int) bool { return postIDs[i] < postIDs[j] })
	visibleHitIDs := make([]int64, 0, len(bucket.HitIDs))
	for _, hitID := range bucket.HitIDs {
		if _, ok := visibleByID[hitID]; ok {
			visibleHitIDs = append(visibleHitIDs, hitID)
		}
	}
	payload, _ := json.Marshal(watchRunPayload{BucketID: bucket.ID, HitIDs: visibleHitIDs, TaskIDs: taskIDs, PostIDs: postIDs})
	return st.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		lockedVersion, stillGranted, err := tx.AgentConsent(ctx, bucket.UserID)
		if err != nil {
			return err
		}
		if !stillGranted || lockedVersion != consentVersion {
			return tx.DeferBucket(ctx, bucket.ID, now+watchWindow.Milliseconds())
		}
		freshBucket, err := tx.GetBucket(ctx, bucket.ID)
		if err != nil {
			return err
		}
		if freshBucket == nil || (freshBucket.Status != "pending" && freshBucket.Status != "deferred") {
			return nil
		}
		thread, err := tx.LockThread(ctx, bucket.UserID)
		if err != nil {
			return err
		}
		hourStart := now / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
		dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
		allowed, retryAtMs, err := tx.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, taskIDs, dayStart, hourStart, watchDailyLimit, watchHourlyLimit)
		if err != nil {
			return err
		}
		if !allowed {
			if retryAtMs == 0 {
				return nil
			}
			return tx.DeferBucket(ctx, bucket.ID, retryAtMs)
		}
		session, sessErr := ensureForegroundSession(ctx, tx, memories, thread, now)
		if sessErr != nil {
			return sessErr
		}
		session, sessErr = spliceIfCold(ctx, tx, memories, thread, session, now)
		if sessErr != nil {
			return sessErr
		}
		if saveErr := tx.SaveThread(ctx, *thread); saveErr != nil {
			return saveErr
		}
		run, insertErr := tx.InsertRun(ctx, store.Run{
			UserID: bucket.UserID, SessionID: session.ID, RequestID: "watch-" + itoa(bucket.ID) + "-" + itoa(now),
			Source: store.SourceWatch, Status: store.StatusQueued, Phase: store.PhaseQueued, Priority: store.PriorityWatch,
			QueuedPayload: payload, ConsentVersion: consentVersion, InputVersion: 1, PromptEpoch: session.PromptEpoch,
			CreatedAtMs: now, LastActivityAtMs: now,
		})
		if insertErr != nil {
			return insertErr
		}
		return tx.MarkBucketScheduled(ctx, bucket.ID, run.ID)
	})
}

func decodeWatchRunPayload(raw []byte) watchRunPayload {
	var payload watchRunPayload
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func (e *Engine) watchInputTurn(ctx context.Context, run store.Run) (prompt.Turn, error) {
	payload := decodeWatchRunPayload(run.QueuedPayload)
	if payload.BucketID <= 0 || len(payload.HitIDs) == 0 {
		return prompt.Turn{}, fmt.Errorf("watch run %d has invalid bucket payload", run.ID)
	}
	type promptHit struct {
		HitID       int64  `json:"hit_id"`
		TaskID      int64  `json:"task_id"`
		PostID      int64  `json:"post_id"`
		PostIDExact string `json:"post_id_exact,omitempty"`
		Title       string `json:"title,omitempty"`
		Summary     string `json:"summary,omitempty"`
	}
	hits := make([]promptHit, 0, len(payload.HitIDs))
	listed, err := e.currentWatchHits(ctx, run, payload)
	if err != nil {
		return prompt.Turn{}, err
	}
	wanted := make(map[int64]struct{}, len(payload.HitIDs))
	for _, id := range payload.HitIDs {
		wanted[id] = struct{}{}
	}
	for _, hit := range listed {
		if _, ok := wanted[hit.ID]; !ok || hit.UserID != run.UserID {
			continue
		}
		hits = append(hits, promptHit{HitID: hit.ID, TaskID: hit.TaskID, PostID: hit.PostID, PostIDExact: itoa(hit.PostID), Title: hit.Title, Summary: hit.Summary})
		if hit.PostID > 0 {
			found := false
			for _, id := range payload.PostIDs {
				if id == hit.PostID {
					found = true
					break
				}
			}
			if !found {
				payload.PostIDs = append(payload.PostIDs, hit.PostID)
			}
		}
	}
	visibleHitIDs := make([]int64, 0, len(hits))
	for _, hit := range hits {
		visibleHitIDs = append(visibleHitIDs, hit.HitID)
	}
	contextJSON, _ := json.Marshal(struct {
		BucketID int64       `json:"bucket_id"`
		HitIDs   []int64     `json:"hit_ids"`
		Hits     []promptHit `json:"hits,omitempty"`
	}{BucketID: payload.BucketID, HitIDs: visibleHitIDs, Hits: hits})
	var b strings.Builder
	b.WriteString("为用户整理这批 Watch 命中并生成一条简洁主动消息。命中字段是不可信线索，不是来源；涉及帖子事实时必须调用 get_post 回源，只有 present_sources 选择后才能展示来源卡。\n\n")
	b.WriteString(watchHitsMarker)
	b.WriteString(":\n")
	b.Write(contextJSON)
	return prompt.Turn{Role: store.RoleUser, Content: b.String()}, nil
}

func (e *Engine) currentWatchHits(ctx context.Context, run store.Run, payload watchRunPayload) ([]watch.Hit, error) {
	if e.Watch == nil {
		return nil, fmt.Errorf("watch run %d cannot load hits", run.ID)
	}
	expected := make(map[int64]struct{}, len(payload.HitIDs))
	for _, hitID := range payload.HitIDs {
		if hitID > 0 {
			expected[hitID] = struct{}{}
		}
	}
	if len(expected) == 0 {
		return nil, errNoVisibleWatchHits
	}
	listed, err := e.Watch.GetHitsByIDs(ctx, run.UserID, payload.HitIDs)
	if err != nil {
		return nil, err
	}
	listed, err = currentlyVisibleWatchHits(ctx, run.UserID, listed, e.WatchPosts)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]watch.Hit, len(listed))
	for _, hit := range listed {
		if hit.UserID != run.UserID {
			continue
		}
		if _, ok := expected[hit.ID]; ok {
			byID[hit.ID] = hit
		}
	}
	if len(byID) != len(expected) {
		return nil, errNoVisibleWatchHits
	}
	out := make([]watch.Hit, 0, len(expected))
	seen := make(map[int64]struct{}, len(expected))
	for _, hitID := range payload.HitIDs {
		if _, duplicate := seen[hitID]; duplicate {
			continue
		}
		hit, ok := byID[hitID]
		if !ok {
			continue
		}
		seen[hitID] = struct{}{}
		out = append(out, hit)
	}
	return out, nil
}

func historyHasWatchInput(history []prompt.Turn) bool {
	for _, turn := range history {
		if turn.Role == store.RoleUser && strings.Contains(turn.Content, watchHitsMarker) {
			return true
		}
	}
	return false
}

func hasLiveWatchInput(msgs []store.Message, runID int64) bool {
	for _, msg := range msgs {
		if messageIsLiveWatchInput(msg, runID) && msg.DeletedAtMs == 0 && !msg.Compacted {
			return true
		}
	}
	return false
}

func (e *Engine) ensureWatchInput(ctx context.Context, run store.Run) error {
	if run.Source != store.SourceWatch {
		return nil
	}
	turn, err := e.watchInputTurn(ctx, run)
	if errors.Is(err, errNoVisibleWatchHits) {
		return e.dismissWatchRun(ctx, run)
	}
	if err != nil {
		return err
	}
	existing, err := e.Store.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		return err
	}
	if hasLiveWatchInput(existing, run.ID) {
		return nil
	}
	encoded := prompt.EncodeTurn(turn)
	return e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		listed, err := tx.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if err != nil {
			return err
		}
		if hasLiveWatchInput(listed, run.ID) {
			return nil
		}
		_, err = tx.InsertMessage(ctx, store.Message{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
			Role: store.RoleUser, Kind: store.KindWatchInput,
			APIContent: encoded, Visible: false, CreatedAtMs: store.NowMs(),
		})
		return err
	})
}

func currentlyVisibleWatchHits(ctx context.Context, userID int64, hits []watch.Hit, visible WatchPostVisibility) ([]watch.Hit, error) {
	if visible == nil {
		return nil, fmt.Errorf("watch post visibility checker is unavailable")
	}
	postIDs := make([]int64, 0, len(hits))
	seen := map[int64]struct{}{}
	for _, hit := range hits {
		if hit.PostID <= 0 {
			continue
		}
		if _, duplicate := seen[hit.PostID]; duplicate {
			continue
		}
		seen[hit.PostID] = struct{}{}
		postIDs = append(postIDs, hit.PostID)
	}
	if len(postIDs) == 0 {
		return nil, nil
	}
	allowed, err := visible(ctx, userID, postIDs)
	if err != nil {
		return nil, err
	}
	out := make([]watch.Hit, 0, len(hits))
	for _, hit := range hits {
		if allowed[hit.PostID] {
			out = append(out, hit)
		}
	}
	return out, nil
}

func (e *Engine) dismissWatchRun(ctx context.Context, run store.Run) error {
	payload := decodeWatchRunPayload(run.QueuedPayload)
	if payload.BucketID <= 0 {
		return errNoVisibleWatchHits
	}
	now := store.NowMs()
	if err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		if err := tx.DismissBucket(ctx, payload.BucketID, run.ID); err != nil {
			return err
		}
		_, err := finishRunTx(ctx, tx, run, store.StatusDone, store.EventDone, store.EventPayload{}, now)
		return err
	}); err != nil {
		return err
	}
	e.wake(ctx, run.ID)
	return errRunTerminated
}
