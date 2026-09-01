package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

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
	defaultMaxResultBytes         = 32 << 10
)

const (
	EffectRead  = "read"
	EffectWrite = "write"

	IdempotencyNone    = "none"
	IdempotencyRequest = "request"
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
	WatchPostIDs   []int64
	LiveMessageIDs []int64
	ChangeIDs      []int64
	Fence          store.LeaseFence
	Recovery       bool
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
	Metadata    Metadata
	executor    executorFunc
	prepare     prepareFunc
}

type Metadata struct {
	Effect         string
	Sources        []string
	MinConsent     int32
	Confirmation   bool
	Available      bool
	Idempotency    string
	MaxResultBytes int
	Poller         bool
}

type UnavailableError struct {
	Tool string
}

func (e *UnavailableError) Error() string { return "agent tool is unavailable" }

type executorFunc func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error)
type prepareFunc func(ctx context.Context, session *Session, argsJSON string) (string, error)

type Registry struct {
	definitions       []Definition
	frozenDefinitions []prompt.ToolDef
	frozen            bool
	executors         map[string]executorFunc
	preparers         map[string]prepareFunc
	metadata          map[string]Metadata
	allowed           map[string]struct{}
	store             store.Store
}

func NewRegistry(clients Clients, allowed []string) (*Registry, error) {
	defs := allDefinitions(clients)
	reg := &Registry{
		executors: make(map[string]executorFunc, len(defs)),
		preparers: map[string]prepareFunc{},
		metadata:  map[string]Metadata{},
		allowed:   map[string]struct{}{},
		store:     clients.Store,
	}
	for _, def := range defs {
		reg.executors[def.Name] = def.executor
		reg.metadata[def.Name] = def.Metadata
		if def.prepare != nil {
			reg.preparers[def.Name] = def.prepare
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
	return &Registry{
		definitions: r.definitions, frozenDefinitions: filterFrozenDefinitions(r.frozenDefinitions, allowed),
		frozen: r.frozen, executors: r.executors, preparers: r.preparers, metadata: r.metadata, allowed: allowed, store: r.store,
	}
}

func (r *Registry) ResolveDefinitions(defs []prompt.ToolDef) *Registry {
	if r == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) != "" {
			allowed[def.Name] = struct{}{}
		}
	}
	return &Registry{
		definitions: r.definitions, frozenDefinitions: append([]prompt.ToolDef(nil), defs...),
		frozen: true, executors: r.executors, preparers: r.preparers, metadata: r.metadata, allowed: allowed, store: r.store,
	}
}

func (r *Registry) Prepare(ctx context.Context, session *Session, name, argsJSON string) (string, error) {
	argsJSON = canonical.UnwrapArgsJSON(argsJSON)
	if r == nil || !r.Has(name) || !r.currentlyAuthorized(session, name) {
		return "", errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	canonicalArgs, err := r.validateArguments(name, argsJSON)
	if err != nil {
		return "", errx.New(errx.ParamError, "tool arguments are invalid")
	}
	prepare := r.preparers[name]
	if prepare == nil {
		return canonicalArgs, nil
	}
	return prepare(ctx, session, canonicalArgs)
}

func RestrictToolsForConsent(registry *Registry, consentVersion int32) *Registry {
	if registry == nil || consentVersion >= CurrentConsentVersion {
		return registry
	}
	return registry.Restrict(Version1Tools())
}

func ForSource(registry *Registry, source string, consentVersion int32) *Registry {
	if registry == nil {
		return nil
	}
	if registry.frozen {
		names := make([]string, 0, len(registry.frozenDefinitions))
		for _, def := range registry.frozenDefinitions {
			if _, configured := registry.allowed[def.Name]; !configured {
				continue
			}
			if len(def.Sources) == 0 {
				meta, ok := registry.metadata[def.Name]
				if ok && consentVersion >= meta.MinConsent && containsString(meta.Sources, source) {
					names = append(names, def.Name)
				}
				continue
			}
			if consentVersion >= def.MinConsent && containsString(def.Sources, source) {
				names = append(names, def.Name)
			}
		}
		return registry.Restrict(names)
	}
	names := make([]string, 0, len(registry.metadata))
	for name, meta := range registry.metadata {
		if !meta.Available || consentVersion < meta.MinConsent || !containsString(meta.Sources, source) {
			continue
		}
		if _, configured := registry.allowed[name]; configured {
			names = append(names, name)
		}
	}
	return registry.Restrict(names)
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
	current := r.metadata[name].Confirmation
	frozen, ok := r.frozenMetadata(name)
	return current || (ok && frozen.Confirmation)
}

func (r *Registry) SideEffect(name string) bool {
	if r == nil {
		return false
	}
	current := r.metadata[name].Effect == EffectWrite
	frozen, ok := r.frozenMetadata(name)
	return current || (ok && frozen.Effect == EffectWrite)
}

func (r *Registry) Poller(name string) bool {
	if r == nil || !r.metadata[name].Poller {
		return false
	}
	frozen, ok := r.frozenMetadata(name)
	return !ok || frozen.Poller
}

func (r *Registry) Metadata(name string) (Metadata, bool) {
	if r == nil {
		return Metadata{}, false
	}
	meta, ok := r.metadata[name]
	return meta, ok
}

func (r *Registry) Definitions() []prompt.ToolDef {
	if r == nil {
		return nil
	}
	if r.frozen {
		return filterFrozenDefinitions(r.frozenDefinitions, r.allowed)
	}
	out := make([]prompt.ToolDef, 0)
	for _, def := range r.definitions {
		if _, ok := r.allowed[def.Name]; !ok {
			continue
		}
		meta := def.Metadata
		if !meta.Available {
			continue
		}
		out = append(out, prompt.ToolDef{
			Name: def.Name, Description: def.Description, Parameters: def.Parameters,
			Effect: meta.Effect, Sources: append([]string(nil), meta.Sources...), MinConsent: meta.MinConsent,
			Confirmation: meta.Confirmation, Idempotency: meta.Idempotency,
			MaxResultBytes: meta.MaxResultBytes, Poller: meta.Poller,
		})
	}
	return out
}

func (r *Registry) Call(ctx context.Context, session *Session, name, callID, argsJSON string) (string, []store.SourceRef, error) {
	if !r.Has(name) || !r.currentlyAuthorized(session, name) {
		return "", nil, errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	if _, err := r.validateArguments(name, argsJSON); err != nil {
		return "", nil, errx.New(errx.ParamError, "tool arguments are invalid")
	}
	meta, metaOK := r.metadata[name]
	handle, ok := r.executors[name]
	if !ok || !metaOK || !meta.Available || handle == nil {
		return unavailableResult(name), nil, &UnavailableError{Tool: name}
	}
	text, sources, err := handle(ctx, session, callID, argsJSON)
	if err != nil {
		return "", nil, err
	}
	limit := r.resultLimit(name, meta.MaxResultBytes)
	if name == PresentSources {
		return limitResult(text, limit), sources, nil
	}
	if len(sources) > 0 {
		if r.store == nil || session == nil || session.RunID <= 0 {
			return "", nil, errx.New(errx.ServiceUnavailable, "source ledger unavailable")
		}
		text, err = r.bindSources(ctx, session, sources, text)
		if err != nil {
			return "", nil, err
		}
	}
	return limitResult(text, limit), nil, nil
}

// validateArguments applies the frozen tool schema before any executor sees
// model-controlled JSON. Executors still decode into their concrete structs,
// but this central gate guarantees unknown fields, trailing values and basic
// type mismatches are handled consistently for every tool.
func (r *Registry) validateArguments(name, raw string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("nil tool registry")
	}
	raw = canonical.UnwrapArgsJSON(raw)
	if raw == "" {
		raw = "{}"
	}
	value, err := decodeStrictValue(raw)
	if err != nil {
		return "", err
	}
	definition, ok := r.definition(name)
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if err := validateSchemaValue(value, definition.Parameters, "$"); err != nil {
		return "", err
	}
	// A recovered run uses the schema captured in its prompt epoch. Validate
	// against the current definition as well so removed fields cannot silently
	// reach a newer executor implementation.
	for _, frozen := range r.frozenDefinitions {
		if frozen.Name != name || reflect.DeepEqual(frozen.Parameters, definition.Parameters) {
			continue
		}
		if err := validateSchemaValue(value, frozen.Parameters, "$"); err != nil {
			return "", err
		}
	}
	canonicalValue, err := canonical.JSON(value)
	if err != nil {
		return "", err
	}
	return string(canonicalValue), nil
}

func (r *Registry) definition(name string) (Definition, bool) {
	for _, definition := range r.definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

func decodeStrictValue(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func validateSchemaValue(value any, schema map[string]any, path string) error {
	if schema == nil {
		return nil
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties := schemaProperties(schema["properties"])
		for key, item := range object {
			property, exists := properties[key]
			if !exists {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
			propertySchema, ok := property.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s has an invalid schema", path, key)
			}
			if err := validateSchemaValue(item, propertySchema, path+"."+key); err != nil {
				return err
			}
		}
		for _, required := range schemaRequired(schema["required"]) {
			if _, exists := object[required]; !exists {
				return fmt.Errorf("%s.%s is required", path, required)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateSchemaValue(item, itemSchema, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "integer":
		if !isJSONInteger(value) {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "number":
		if !isJSONNumber(value) {
			return fmt.Errorf("%s must be a number", path)
		}
	}
	if enum, ok := schema["enum"]; ok && !enumContains(enum, value) {
		return fmt.Errorf("%s has an invalid value", path)
	}
	return nil
}

func schemaProperties(raw any) map[string]any {
	if properties, ok := raw.(map[string]any); ok {
		return properties
	}
	return map[string]any{}
}

func schemaRequired(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func isJSONInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := number.Int64()
	return err == nil
}

func isJSONNumber(value any) bool {
	if number, ok := value.(json.Number); ok {
		_, err := number.Float64()
		return err == nil
	}
	return false
}

func enumContains(raw, value any) bool {
	switch values := raw.(type) {
	case []string:
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, candidate := range values {
			if candidate == text {
				return true
			}
		}
	case []any:
		for _, candidate := range values {
			if fmt.Sprint(candidate) == fmt.Sprint(value) {
				return true
			}
		}
	}
	return false
}

func (r *Registry) currentlyAuthorized(session *Session, name string) bool {
	if r == nil {
		return false
	}
	meta, ok := r.metadata[name]
	if !ok {
		return false
	}
	source := store.SourceUser
	consent := CurrentConsentVersion
	if session != nil {
		if strings.TrimSpace(session.Source) != "" {
			source = session.Source
		}
		if session.ConsentVersion > 0 {
			consent = session.ConsentVersion
		}
	}
	return consent >= meta.MinConsent && containsString(meta.Sources, source)
}

func (r *Registry) frozenMetadata(name string) (Metadata, bool) {
	if r == nil || !r.frozen {
		return Metadata{}, false
	}
	for _, def := range r.frozenDefinitions {
		if def.Name != name {
			continue
		}
		return Metadata{
			Effect: def.Effect, Sources: append([]string(nil), def.Sources...), MinConsent: def.MinConsent,
			Confirmation: def.Confirmation, Idempotency: def.Idempotency,
			MaxResultBytes: def.MaxResultBytes, Poller: def.Poller,
		}, true
	}
	return Metadata{}, false
}

func (r *Registry) resultLimit(name string, current int) int {
	frozen, ok := r.frozenMetadata(name)
	if !ok || frozen.MaxResultBytes <= 0 {
		return current
	}
	if current <= 0 || frozen.MaxResultBytes < current {
		return frozen.MaxResultBytes
	}
	return current
}

func (r *Registry) bindSources(ctx context.Context, session *Session, sources []store.SourceRef, text string) (string, error) {
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n来源 handle（仅可对本 run 使用 present_sources）：")
	for _, src := range sources {
		handle := src.Handle
		if handle == "" {
			handle = randomHandle()
		}
		payload, _ := json.Marshal(src)
		insert := func(ctx context.Context, target store.Store) error {
			_, err := target.InsertSource(ctx, store.Source{
				RunID: session.RunID, Handle: handle, Kind: src.Kind, AuthorityID: src.AuthorityID,
				Revision: src.Revision, PayloadJSON: string(payload), CreatedAtMs: store.NowMs(),
			})
			return err
		}
		var err error
		if session.Fence.Generation > 0 {
			err = r.store.RunStep(ctx, session.Fence, insert)
		} else {
			err = insert(ctx, r.store)
		}
		if err != nil {
			logx.WithContext(ctx).Errorw("source ledger insert failed", logx.Field("err", err.Error()))
			return "", fmt.Errorf("source ledger insert: %w", err)
		}
		fmt.Fprintf(&b, "\n- %s (%s)", handle, src.Kind)
	}
	return b.String(), nil
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
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("tool arguments contain a trailing JSON value")
		}
		return err
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
	defs := []Definition{
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
		{Name: UpdateWatchTask, Description: "启用或停用追踪。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}, "enabled": map[string]any{"type": "boolean"}, "expected_version": map[string]any{"type": "integer"}}, []string{"id", "enabled", "expected_version"}), executor: updateWatchTaskExecutor(clients.Watch)},
		{Name: DeleteWatchTask, Description: "删除追踪任务。", Parameters: objectSchema(map[string]any{"id": map[string]any{"type": "integer"}, "expected_version": map[string]any{"type": "integer"}}, []string{"id", "expected_version"}), executor: deleteWatchTaskExecutor(clients.Watch)},
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
		}, []string{"post_id"}), executor: updatePostExecutor(clients.Content, clients.Media), prepare: postRevisionPreparer(clients.Content)},
		{Name: DeletePost, Description: "删除本人帖子，执行前需用户逐次确认。", Parameters: objectSchema(map[string]any{
			"post_id": map[string]any{"type": "integer"}, "expected_revision": map[string]any{"type": "integer"},
		}, []string{"post_id"}), executor: deletePostExecutor(clients.Content), prepare: postRevisionPreparer(clients.Content)},
		{Name: SearchHistory, Description: "检索当前用户不在 live context 中的 Assistant 历史。shape=keywords|around|session|recent。", Parameters: objectSchema(map[string]any{
			"shape": map[string]any{"type": "string", "enum": []string{"keywords", "around", "session", "recent"}}, "query": map[string]any{"type": "string"},
			"message_id": map[string]any{"type": "integer"}, "session_id": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"},
		}, []string{"shape"}), executor: searchHistoryExecutor(clients.History)},
		{Name: PresentSources, Description: "把本 run 已验证的至多 10 个 source handle 展示为 source_card。", Parameters: objectSchema(map[string]any{
			"handles": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}, []string{"handles"}), executor: presentSourcesExecutor(clients)},
	}
	decorateDefinitions(defs, clients)
	return defs
}

func decorateDefinitions(defs []Definition, clients Clients) {
	writeTools := stringSet(CreatePost, UpdatePost, DeletePost, AddMemory, ReplaceMemory, RemoveMemory, BatchMemory,
		CreateWatchTask, UpdateWatchTask, DeleteWatchTask)
	versionOne := stringSet(Version1Tools()...)
	watchTools := stringSet(WatchTools()...)
	reviewTools := stringSet(ReviewTools()...)
	for i := range defs {
		def := &defs[i]
		meta := Metadata{
			Effect: EffectRead, Sources: []string{store.SourceUser}, MinConsent: CurrentConsentVersion,
			Available: definitionAvailable(def.Name, clients), Idempotency: IdempotencyNone,
			MaxResultBytes: defaultMaxResultBytes,
		}
		if _, ok := writeTools[def.Name]; ok {
			meta.Effect = EffectWrite
			meta.Idempotency = IdempotencyRequest
		}
		if _, ok := versionOne[def.Name]; ok {
			meta.MinConsent = 1
		}
		if _, ok := watchTools[def.Name]; ok {
			meta.Sources = append(meta.Sources, store.SourceWatch)
		}
		if _, ok := reviewTools[def.Name]; ok {
			meta.Sources = append(meta.Sources, store.SourceMemoryReview)
		}
		meta.Confirmation = def.Name == DeletePost
		def.Metadata = meta
	}
}

func definitionAvailable(name string, clients Clients) bool {
	switch name {
	case SearchPosts:
		return nonNil(clients.Search) && nonNil(clients.Content)
	case SearchUsers, SearchTags:
		return nonNil(clients.Search)
	case GetPost, GetPostComments, ComparePosts, GetMyPosts, UpdatePost, DeletePost:
		return nonNil(clients.Content)
	case GetMemory, AddMemory, ReplaceMemory, RemoveMemory, BatchMemory:
		return nonNil(clients.Memory)
	case RecommendPosts, SimilarPosts:
		return nonNil(clients.Recommend) && nonNil(clients.Content)
	case GetMyFavorites, GetMyLikes:
		return nonNil(clients.Interaction) && nonNil(clients.Content)
	case GetMyFollowing:
		return nonNil(clients.User)
	case ListWatchTasks, UpdateWatchTask, DeleteWatchTask:
		return nonNil(clients.Watch)
	case CreateWatchTask:
		return nonNil(clients.Watch)
	case WebSearch:
		return nonNil(clients.Web)
	case CreatePost:
		return nonNil(clients.Content) && nonNil(clients.Media)
	case SearchHistory:
		return nonNil(clients.History)
	case PresentSources:
		return nonNil(clients.Store) && nonNil(clients.Content)
	default:
		return false
	}
}

func nonNil(value any) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !rv.IsNil()
	default:
		return true
	}
}

func unavailableResult(name string) string {
	raw, _ := json.Marshal(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": "TOOL_UNAVAILABLE", "tool": name, "message": "工具当前不可用，请改用其它能力或直接说明限制。"},
	})
	return string(raw)
}

func limitResult(text string, limit int) string {
	if limit <= 0 {
		limit = defaultMaxResultBytes
	}
	if len(text) <= limit {
		return text
	}
	const reserve = 256
	maxText := limit - reserve
	if maxText < 0 {
		maxText = 0
	}
	truncated := truncateUTF8Bytes(text, maxText)
	raw, _ := json.Marshal(map[string]any{
		"ok": true, "truncated": true, "original_bytes": len(text), "text": truncated,
	})
	if len(raw) <= limit {
		return string(raw)
	}
	return `{"ok":true,"truncated":true,"text":""}`
}

func truncateUTF8Bytes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

func filterFrozenDefinitions(defs []prompt.ToolDef, allowed map[string]struct{}) []prompt.ToolDef {
	out := make([]prompt.ToolDef, 0, len(defs))
	for _, def := range defs {
		if _, ok := allowed[def.Name]; ok {
			out = append(out, def)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
