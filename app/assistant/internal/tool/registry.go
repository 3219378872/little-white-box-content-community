package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/websearch"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	SearchPosts     = "search_posts"
	SearchUsers     = "search_users"
	SearchTags      = "search_tags"
	GetPost         = "get_post"
	GetPostComments = "get_post_comments"
	GetMemory       = "read_memory"
	AddMemory       = "add_memory"
	ReplaceMemory   = "replace_memory"
	RemoveMemory    = "remove_memory"
	BatchMemory     = "batch_memory"
	RecommendPosts  = "recommend_posts"
	SimilarPosts    = "similar_posts"
	ComparePosts    = "compare_posts"
	GetMyFavorites  = "get_my_favorites"
	GetMyLikes      = "get_my_likes"
	GetMyFollowing  = "get_my_following"
	GetMyPosts      = "get_my_posts"
	CreateWatchTask = "create_watch_task"
	ListWatchTasks  = "list_watch_tasks"
	UpdateWatchTask = "update_watch_task"
	DeleteWatchTask = "delete_watch_task"
	WebSearch       = "web_search"
	CreatePost      = "create_post"
	UpdatePost      = "update_post"
	DeletePost      = "delete_post"
	SearchHistory   = "search_history"
	PresentSources  = "present_sources"

	CurrentConsentVersion   int32 = 2
	maxEvidenceSnippetRunes       = 360
	defaultPageResult             = 5
	maxTitleRunes                 = 120
	maxContentRunes               = 20000
	maxTags                       = 10
	maxTagRunes                   = 32
	maxPostImages                 = 9
	publishedPostStatus     int32 = 1
	commentActiveStatus     int32 = 1
)

func Version1Tools() []string {
	return []string{SearchPosts, WebSearch, CreatePost, UpdatePost, DeletePost}
}

func WatchTools() []string {
	return []string{SearchPosts, SearchUsers, SearchTags, GetPost, GetPostComments, RecommendPosts, SimilarPosts, ComparePosts, GetMemory, SearchHistory, PresentSources}
}

func ReviewTools() []string {
	return []string{GetMemory, AddMemory, ReplaceMemory, RemoveMemory, BatchMemory}
}

type Clients struct {
	Search      searchservice.SearchService
	Content     contentservice.ContentService
	Media       mediaservice.MediaService
	Recommend   recommendservice.RecommendService
	Interaction interactionservice.InteractionService
	User        userservice.UserService
	Web         websearch.Searcher
	Memory      memory.Store
	Watch       watch.Store
	Store       store.Store
	History     History
}

type Attachment struct {
	MediaID int64
	URL     string
}

type Session struct {
	UserID         int64
	SessionID      int64
	RunID          int64
	RequestID      string
	Source         string
	ConsentVersion int32
	Attachments    []Attachment
	ContextPostID  int64
}

type History interface {
	Search(ctx context.Context, sess *Session, args HistoryArgs) (string, error)
}

type HistoryArgs struct {
	Shape     string `json:"shape"`
	Query     string `json:"query"`
	MessageID int64  `json:"message_id"`
	SessionID int64  `json:"session_id"`
	Limit     int    `json:"limit"`
}

type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	HighRisk    bool
	executor    executorFunc
}

type executorFunc func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error)

type Registry struct {
	definitions []Definition
	executors   map[string]executorFunc
	highRisk    map[string]struct{}
	allowed     map[string]struct{}
	store       store.Store
}

func NewRegistry(clients Clients, allowed []string) (*Registry, error) {
	defs := allDefinitions(clients)
	reg := &Registry{
		executors: make(map[string]executorFunc, len(defs)),
		highRisk:  map[string]struct{}{},
		allowed:   map[string]struct{}{},
		store:     clients.Store,
	}
	for _, def := range defs {
		reg.executors[def.Name] = def.executor
		if def.HighRisk {
			reg.highRisk[def.Name] = struct{}{}
		}
		if len(allowed) == 0 {
			reg.allowed[def.Name] = struct{}{}
			continue
		}
		for _, name := range allowed {
			if strings.TrimSpace(name) == def.Name {
				reg.allowed[def.Name] = struct{}{}
			}
		}
	}
	if len(reg.allowed) == 0 {
		return nil, fmt.Errorf("agent: AllowedTools must contain at least one tool")
	}
	reg.definitions = defs
	return reg, nil
}

func (r *Registry) Restrict(names []string) *Registry {
	if r == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, name := range names {
		if r.Has(name) {
			allowed[name] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	return &Registry{definitions: r.definitions, executors: r.executors, highRisk: r.highRisk, allowed: allowed, store: r.store}
}

func RestrictToolsForConsent(registry *Registry, consentVersion int32) *Registry {
	if registry == nil || consentVersion >= CurrentConsentVersion {
		return registry
	}
	return registry.Restrict(Version1Tools())
}

func ForSource(registry *Registry, source string, consentVersion int32) *Registry {
	switch source {
	case store.SourceWatch:
		return registry.Restrict(WatchTools())
	case store.SourceMemoryReview:
		return registry.Restrict(ReviewTools())
	default:
		return RestrictToolsForConsent(registry, consentVersion)
	}
}

func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.allowed[name]
	return ok
}

func (r *Registry) HighRisk(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.highRisk[name]
	return ok
}

func (r *Registry) Definitions() []prompt.ToolDef {
	if r == nil {
		return nil
	}
	out := make([]prompt.ToolDef, 0)
	for _, def := range r.definitions {
		if _, ok := r.allowed[def.Name]; !ok {
			continue
		}
		out = append(out, prompt.ToolDef{Name: def.Name, Description: def.Description, Parameters: def.Parameters})
	}
	return out
}

func (r *Registry) Call(ctx context.Context, session *Session, name, callID, argsJSON string) (string, []store.SourceRef, error) {
	if !r.Has(name) {
		return "", nil, errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	handle, ok := r.executors[name]
	if !ok {
		return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	text, sources, err := handle(ctx, session, callID, argsJSON)
	if err != nil {
		return "", nil, err
	}
	if name == PresentSources {
		return text, sources, nil
	}
	if len(sources) > 0 && r.store != nil && session != nil {
		text = r.bindSources(ctx, session, sources, text)
	}
	return text, nil, nil
}

func (r *Registry) bindSources(ctx context.Context, session *Session, sources []store.SourceRef, text string) string {
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n来源 handle（仅可对本 run 使用 present_sources）：")
	for _, src := range sources {
		handle := src.Handle
		if handle == "" {
			handle = randomHandle()
		}
		payload, _ := json.Marshal(src)
		_, err := r.store.InsertSource(ctx, store.Source{
			RunID: session.RunID, Handle: handle, Kind: src.Kind, AuthorityID: src.AuthorityID,
			Revision: src.Revision, PayloadJSON: string(payload), CreatedAtMs: store.NowMs(),
		})
		if err != nil {
			logx.WithContext(ctx).Infow("source ledger insert failed", logx.Field("err", err.Error()))
			continue
		}
		fmt.Fprintf(&b, "\n- %s (%s)", handle, src.Kind)
	}
	return b.String()
}

func randomHandle() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "src_" + hex.EncodeToString(buf[:])
}

func publishedPosts(ctx context.Context, client contentservice.ContentService, ids []int64) (map[int64]*contentservice.PostInfo, error) {
	return visibility.PublishedByIDs(ctx, client, ids)
}

func strictUnmarshal(raw string, target any) error {
	raw = canonical.UnwrapArgsJSON(raw)
	if raw == "" {
		raw = "{}"
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return json.Unmarshal([]byte(raw), target)
	}
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func sessionUserID(session *Session) (int64, error) {
	if session == nil || session.UserID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return session.UserID, nil
}

func CanonicalDigest(argsJSON string) (string, error) {
	return canonical.DigestArgs(argsJSON)
}

func allDefinitions(clients Clients) []Definition {
	return []Definition{
		{Name: SearchPosts, Description: "搜索站内已发布帖子。结果以 source handle 返回。", Parameters: objectSchema(map[string]any{
			"keyword": map[string]any{"type": "string"}, "page": map[string]any{"type": "integer"},
			"page_size": map[string]any{"type": "integer"}, "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"sort_by": map[string]any{"type": "integer"}, "include_comments": map[string]any{"type": "boolean"}, "seed_post_id": map[string]any{"type": "integer"},
		}, []string{"keyword"}), executor: searchPostsExecutor(clients)},
		{Name: SearchUsers, Description: "搜索公开用户，结果不是社区来源。", Parameters: objectSchema(map[string]any{
			"keyword": map[string]any{"type": "string"}, "page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"},
		}, []string{"keyword"}), executor: searchUsersExecutor(clients.Search)},
		{Name: SearchTags, Description: "搜索标签。", Parameters: objectSchema(map[string]any{
			"keyword": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"},
		}, []string{"keyword"}), executor: searchTagsExecutor(clients.Search)},
		{Name: GetPost, Description: "按 ID 读取当前用户可见的已发布帖子。", Parameters: objectSchema(map[string]any{"post_id": map[string]any{"type": "integer"}}, []string{"post_id"}), executor: getPostExecutor(clients.Content)},
		{Name: GetPostComments, Description: "读取已发布帖子下的有效评论。", Parameters: objectSchema(map[string]any{
			"post_id": map[string]any{"type": "integer"}, "page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"},
		}, []string{"post_id"}), executor: getPostCommentsExecutor(clients.Content)},
		{Name: GetMemory, Description: "读取当前用户 MEMORY/USER 自然语言条目。", Parameters: objectSchema(map[string]any{"target": map[string]any{"type": "string", "enum": []string{"memory", "user"}}}, nil), executor: readMemoryExecutor(clients.Memory)},
		{Name: AddMemory, Description: "新增一条 MEMORY 或 USER 自然语言条目。", Parameters: objectSchema(map[string]any{"target": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, []string{"target", "content"}), executor: addMemoryExecutor(clients.Memory)},
		{Name: ReplaceMemory, Description: "按 version 替换一条记忆。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}, "content": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer"}}, []string{"id", "content", "version"}), executor: replaceMemoryExecutor(clients.Memory)},
		{Name: RemoveMemory, Description: "按 version 删除一条记忆。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}, "version": map[string]any{"type": "integer"}}, []string{"id", "version"}), executor: removeMemoryExecutor(clients.Memory)},
		{Name: BatchMemory, Description: "原子批量 add/replace/remove。", Parameters: objectSchema(map[string]any{"ops": map[string]any{"type": "array"}}, []string{"ops"}), executor: batchMemoryExecutor(clients.Memory)},
		{Name: RecommendPosts, Description: "取当前用户可见的已发布推荐帖，并登记 source handle。", Parameters: objectSchema(map[string]any{"page_size": map[string]any{"type": "integer"}}, nil), executor: recommendPostsExecutor(clients)},
		{Name: SimilarPosts, Description: "按种子帖取相似已发布帖子。", Parameters: objectSchema(map[string]any{"post_id": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"}}, nil), executor: similarPostsExecutor(clients)},
		{Name: ComparePosts, Description: "比较 2～5 篇已回源帖子。", Parameters: objectSchema(map[string]any{"post_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}, []string{"post_ids"}), executor: comparePostsExecutor(clients.Content)},
		{Name: GetMyFavorites, Description: "列出自己收藏的已发布帖子。", Parameters: objectSchema(map[string]any{"page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"}}, nil), executor: getMyFavoritesExecutor(clients)},
		{Name: GetMyLikes, Description: "列出自己点赞的已发布帖子。", Parameters: objectSchema(map[string]any{"page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"}}, nil), executor: getMyLikesExecutor(clients)},
		{Name: GetMyFollowing, Description: "列出自己关注的人。", Parameters: objectSchema(map[string]any{"page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"}}, nil), executor: getMyFollowingExecutor(clients.User)},
		{Name: GetMyPosts, Description: "列出自己已发布的帖子。", Parameters: objectSchema(map[string]any{"page_size": map[string]any{"type": "integer"}}, nil), executor: getMyPostsExecutor(clients.Content)},
		{Name: ListWatchTasks, Description: "列出条件追踪任务。", Parameters: objectSchema(map[string]any{}, nil), executor: listWatchTasksExecutor(clients.Watch)},
		{Name: CreateWatchTask, Description: "创建条件追踪任务。", Parameters: objectSchema(map[string]any{
			"condition_type": map[string]any{"type": "string"}, "target_type": map[string]any{"type": "string"},
			"target_id": map[string]any{"type": "integer"}, "target_text": map[string]any{"type": "string"},
		}, []string{"condition_type", "target_type"}), executor: createWatchTaskExecutor(clients)},
		{Name: UpdateWatchTask, Description: "启用或停用追踪。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}, "enabled": map[string]any{"type": "boolean"}}, []string{"id", "enabled"}), executor: updateWatchTaskExecutor(clients.Watch)},
		{Name: DeleteWatchTask, Description: "删除追踪任务。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}}, []string{"id"}), executor: deleteWatchTaskExecutor(clients.Watch)},
		{Name: WebSearch, Description: "搜索公共互联网，结果登记为 web source handle。", Parameters: objectSchema(map[string]any{"query": map[string]any{"type": "string"}}, []string{"query"}), executor: webSearchExecutor(clients.Web)},
		{Name: CreatePost, Description: "以当前用户身份创建帖子。", Parameters: objectSchema(map[string]any{
			"title": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
			"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"image_media_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"status":          map[string]any{"type": "integer"},
		}, []string{"title", "content"}), executor: createPostExecutor(clients.Content, clients.Media)},
		{Name: UpdatePost, Description: "更新本人帖子。", Parameters: objectSchema(map[string]any{
			"post_id": map[string]any{"type": "integer"}, "title": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
			"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"image_media_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"status":          map[string]any{"type": "integer"}, "expected_revision": map[string]any{"type": "integer"},
		}, []string{"post_id"}), executor: updatePostExecutor(clients.Content, clients.Media)},
		{Name: DeletePost, Description: "删除本人帖子，执行前需用户逐次确认。", HighRisk: true, Parameters: objectSchema(map[string]any{
			"post_id": map[string]any{"type": "integer"}, "expected_revision": map[string]any{"type": "integer"},
		}, []string{"post_id"}), executor: deletePostExecutor(clients.Content)},
		{Name: SearchHistory, Description: "在当前用户 Assistant 历史中做 BM25 召回。shape=keywords|around|session|recent。", Parameters: objectSchema(map[string]any{
			"shape": map[string]any{"type": "string"}, "query": map[string]any{"type": "string"},
			"message_id": map[string]any{"type": "integer"}, "session_id": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"},
		}, nil), executor: searchHistoryExecutor(clients.History)},
		{Name: PresentSources, Description: "把本 run 已验证的至多 10 个 source handle 展示为 source_card。", Parameters: objectSchema(map[string]any{
			"handles": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}, []string{"handles"}), executor: presentSourcesExecutor(clients.Store)},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
