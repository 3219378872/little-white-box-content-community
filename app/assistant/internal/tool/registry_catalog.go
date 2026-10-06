package tool

import (
	"esx/app/assistant/internal/store"
	"reflect"
)

// allDefinitions 是全部工具的目录：名称、给模型的说明、参数 schema 与执行器。
func allDefinitions(clients Clients) []Definition {
	defs := []Definition{
		{Name: SearchPosts, Description: "搜索站内已发布帖子。结果以 source handle 返回。", Parameters: objectSchema(map[string]any{
			"keyword": map[string]any{"type": "string"}, "page": map[string]any{"type": "integer"},
			"page_size": map[string]any{"type": "integer"}, "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"sort_by": map[string]any{"type": "integer"},
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
	defs = append(defs, researchDefinitions(clients)...)
	decorateDefinitions(defs, clients)
	return defs
}

// decorateDefinitions 统一补齐元数据：写工具按请求幂等，v1 工具只需最低授权版本，
// 记忆工具开放给后台回顾，删帖需逐次确认，研究类工具要求客户端协议 v2。
func decorateDefinitions(defs []Definition, clients Clients) {
	writeTools := stringSet(CreatePost, UpdatePost, DeletePost, AddMemory, ReplaceMemory, RemoveMemory, BatchMemory)
	versionOne := stringSet(Version1Tools()...)
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
		if _, ok := reviewTools[def.Name]; ok {
			meta.Sources = append(meta.Sources, store.SourceMemoryReview)
		}
		meta.Confirmation = def.Name == DeletePost
		if def.Name == AskQuestions || def.Name == ReadSource || def.Name == PublishAnswer {
			meta.MinClientProtocol = 2
		}
		def.Metadata = meta
	}
}

// definitionAvailable 按依赖的下游客户端是否已配置判断工具当前是否可用。
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
	case WebSearch:
		return nonNil(clients.Web)
	case CreatePost:
		return nonNil(clients.Content) && nonNil(clients.Media)
	case SearchHistory:
		return nonNil(clients.History)
	case PresentSources:
		return nonNil(clients.Store) && nonNil(clients.Content)
	case AskQuestions, ReadSource, PublishAnswer:
		return nonNil(clients.Store)
	default:
		return false
	}
}

// nonNil 判断接口值是否真正非空，包括装着 nil 指针的接口。
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
