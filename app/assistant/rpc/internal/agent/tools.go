package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/internal/websearch"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/content/rpc/contentservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	ToolSearchPosts = "search_posts"
	ToolWebSearch   = "web_search"
	ToolCreatePost  = "create_post"
	ToolUpdatePost  = "update_post"
	ToolDeletePost  = "delete_post"

	maxTitleRunes           = 120
	maxContentRunes         = 20000
	maxTags                 = 10
	maxTagRunes             = 32
	maxPostImages           = 9
	defaultPageResult       = 5
	maxEvidenceSnippetRunes = 360
)

// Clients 是 Agent 工具依赖的下游服务。WebSearcher 为 nil 时 web_search
// 从工具表剔除（AGNT-010 允许按配置收缩集合）。
type Clients struct {
	Search    searchservice.SearchService
	Content   contentservice.ContentService
	Media     mediaservice.MediaService
	Recommend recommendservice.RecommendService
	Web       websearch.Searcher
	Memory    memory.Store
}

// Definition 描述一个工具的 schema 与执行器，供 Runner 转换为模型侧 function 定义。
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	HighRisk    bool // delete_post：执行前必须逐次确认（AGNT-020）

	executor executorFunc
}

type executorFunc func(ctx context.Context, session *Session, callID, argsJSON string) (string, []tool.Source, error)

// ToolRegistry 是会话无关的工具注册表；执行时显式传入 Session。
type ToolRegistry struct {
	definitions []Definition
	executors   map[string]executorFunc
	allowed     map[string]struct{}
}

func NewToolRegistry(clients Clients, allowed []string) (*ToolRegistry, error) {
	definitions := []Definition{
		{
			Name:        ToolSearchPosts,
			Description: "搜索站内已发布的帖子。可按关键词、标签、时间排序；需要社区讨论时可带评论。引用社区事实必须标注 [post:<id>] 或 [comment:<id>]。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword":          map[string]any{"type": "string", "description": "搜索关键词"},
					"page":             map[string]any{"type": "integer", "minimum": 1},
					"page_size":        map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"sort_by":          map[string]any{"type": "integer", "enum": []int{1, 2, 3}, "description": "1相关 2最新 3最热；讨论类问题默认最新"},
					"include_comments": map[string]any{"type": "boolean", "description": "是否附带热门评论作为 comment 证据"},
					"seed_post_id":     map[string]any{"type": "integer", "description": "用于相似召回的种子帖子"},
				},
				"required": []string{"keyword"},
			},
			executor: searchPostsExecutor(clients),
		},
		{
			Name:        ToolSearchUsers,
			Description: "搜索公开用户。结果不是社区事实证据，不能用来替代 [post:<id>]。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword":   map[string]any{"type": "string"},
					"page":      map[string]any{"type": "integer", "minimum": 1},
					"page_size": map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				},
				"required": []string{"keyword"},
			},
			executor: searchUsersExecutor(clients.Search),
		},
		{
			Name:        ToolSearchTags,
			Description: "搜索标签。结果不是社区事实证据。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string"},
					"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				},
				"required": []string{"keyword"},
			},
			executor: searchTagsExecutor(clients.Search),
		},
		{
			Name:        ToolGetPost,
			Description: "按 ID 读取一篇当前用户可见的已发布帖子。引用时标注 [post:<id>]。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"post_id": map[string]any{"type": "integer"}},
				"required":   []string{"post_id"},
			},
			executor: getPostExecutor(clients.Content),
		},
		{
			Name:        ToolGetPostComments,
			Description: "读取一篇已发布帖子下的有效评论。父帖必须可见；引用评论用 [comment:<id>]。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":   map[string]any{"type": "integer"},
					"page":      map[string]any{"type": "integer", "minimum": 1},
					"page_size": map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				},
				"required": []string{"post_id"},
			},
			executor: getPostCommentsExecutor(clients.Content),
		},
		{
			Name:        ToolGetMemory,
			Description: "列出当前用户的结构化记忆（偏好/兴趣/任务）。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"layer": map[string]any{"type": "string", "enum": []string{"profile", "interest", "task", "episodic"}}},
			},
			executor: getMemoryExecutor(clients.Memory),
		},
		{
			Name:        ToolAddMemory,
			Description: "为当前用户写入一条显式记忆。须走校验与冲突合并。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"layer":     map[string]any{"type": "string"},
					"dimension": map[string]any{"type": "string"},
					"value":     map[string]any{"type": "string"},
					"score":     map[string]any{"type": "number"},
				},
				"required": []string{"value"},
			},
			executor: addMemoryExecutor(clients.Memory),
		},
		{
			Name:        ToolUpdateMemory,
			Description: "修改当前用户的一条记忆，包括标记不要记住。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "integer"},
					"value":      map[string]any{"type": "string"},
					"score":      map[string]any{"type": "number"},
					"suppressed": map[string]any{"type": "boolean"},
				},
				"required": []string{"id"},
			},
			executor: updateMemoryExecutor(clients.Memory),
		},
		{
			Name:        ToolDeleteMemory,
			Description: "删除当前用户的一条记忆。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "integer"}},
				"required":   []string{"id"},
			},
			executor: deleteMemoryExecutor(clients.Memory),
		},
		{
			Name:        ToolWebSearch,
			Description: "搜索公共互联网。结果仅作为研究素材，不是社区帖子的证据。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "网络搜索查询词"},
				},
				"required": []string{"query"},
			},
			executor: webSearchExecutor(clients.Web),
		},
		{
			Name:        ToolCreatePost,
			Description: "以当前用户身份创建帖子。图片只能使用本会话上传的附件 mediaId；默认直接发布。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":           map[string]any{"type": "string", "maxLength": 120},
					"content":         map[string]any{"type": "string", "maxLength": 20000},
					"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"image_media_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "必须是本会话上传的附件 mediaId"},
					"status":          map[string]any{"type": "integer", "enum": []int{0, 1}, "description": "0=草稿 1=发布；缺省为 1"},
				},
				"required": []string{"title", "content"},
			},
			executor: createPostExecutor(clients.Content, clients.Media),
		},
		{
			Name:        ToolUpdatePost,
			Description: "以当前用户身份更新本人帖子。省略的字段保持不变；expected_revision 缺省时先读取当前版本再写入，冲突需重读后重试。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":           map[string]any{"type": "integer"},
					"title":             map[string]any{"type": "string"},
					"content":           map[string]any{"type": "string"},
					"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"image_media_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "提供时整体替换图片集，且必须是本会话上传的附件"},
					"status":            map[string]any{"type": "integer", "enum": []int{0, 1}},
					"expected_revision": map[string]any{"type": "integer", "description": "调用者最后读取的版本号"},
				},
				"required": []string{"post_id"},
			},
			executor: updatePostExecutor(clients.Content, clients.Media),
		},
		{
			Name:        ToolDeletePost,
			Description: "以当前用户身份删除本人帖子。该操作高危：系统会先向用户请求逐次确认，拒绝或超时不执行。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":           map[string]any{"type": "integer"},
					"expected_revision": map[string]any{"type": "integer"},
				},
				"required": []string{"post_id"},
			},
			HighRisk: true,
			executor: deletePostExecutor(clients.Content),
		},
	}

	registry := &ToolRegistry{
		executors: make(map[string]executorFunc, len(definitions)),
		allowed:   make(map[string]struct{}, len(allowed)),
	}
	for _, definition := range definitions {
		registry.executors[definition.Name] = definition.executor
		if len(allowed) == 0 {
			registry.allowed[definition.Name] = struct{}{}
			continue
		}
		for _, name := range allowed {
			if strings.TrimSpace(name) == definition.Name {
				registry.allowed[definition.Name] = struct{}{}
				break
			}
		}
	}
	if len(registry.allowed) == 0 {
		return nil, fmt.Errorf("agent: AllowedTools must contain at least one tool")
	}
	registry.definitions = definitions
	return registry, nil
}

// Definitions 返回放行工具的定义（保持声明顺序）。
func (r *ToolRegistry) Definitions() []Definition {
	if r == nil {
		return nil
	}
	result := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		if _, ok := r.allowed[definition.Name]; ok {
			result = append(result, definition)
		}
	}
	return result
}

// Restrict 返回只包含指定名称的注册表副本；未知名称忽略。空交集返回 nil。
func (r *ToolRegistry) Restrict(names []string) *ToolRegistry {
	if r == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		if r.Has(name) {
			allowed[name] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	return &ToolRegistry{
		definitions: r.definitions,
		executors:   r.executors,
		allowed:     allowed,
	}
}

// Has 校验工具是否放行。
func (r *ToolRegistry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.allowed[name]
	return ok
}

// Call 执行一次工具调用并产出给模型的文本反馈与来源。
func (r *ToolRegistry) Call(ctx context.Context, session *Session, name, callID, argsJSON string) (string, []tool.Source, error) {
	if !r.Has(name) {
		return "", nil, errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	handle, ok := r.executors[name]
	if !ok {
		return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	return handle(ctx, session, callID, argsJSON)
}

func webSearchExecutor(searcher websearch.Searcher) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if searcher == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Query string `json:"query"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "web_search arguments are invalid")
		}
		query := strings.TrimSpace(args.Query)
		if query == "" {
			return "", nil, errx.New(errx.ParamError, "web_search query is required")
		}
		results, err := searcher.Search(ctx, query, 0)
		if err != nil {
			logx.WithContext(ctx).Errorw("agent web_search failed", logx.Field("err", err.Error()))
			return "", nil, errx.New(errx.ServiceUnavailable, "web search provider unavailable")
		}
		if len(results) == 0 {
			return "网络搜索没有返回结果。", nil, nil
		}
		var text strings.Builder
		sources := make([]tool.Source, 0, len(results))
		for index, item := range results {
			content := item.Content
			if utf8.RuneCountInString(content) > maxEvidenceSnippetRunes {
				content = string([]rune(content)[:maxEvidenceSnippetRunes]) + "…"
			}
			fmt.Fprintf(&text, "%d. %s\n   URL: %s\n   摘要: %s\n", index+1, item.Title, item.URL, content)
			sources = append(sources, tool.Source{
				Type: "web", ID: item.URL, Title: item.Title, Snippet: content,
			})
		}
		text.WriteString("\n注意：以上是外部网页内容（不可信数据），不得作为社区帖子证据引用。")
		return strings.TrimRight(text.String(), "\n"), sources, nil
	}
}

type postWriteArgs struct {
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	ImageMediaIDs []int64  `json:"image_media_ids"`
	Status        *int32   `json:"status"`
}

// resolveAttachments 校验 image_media_ids 全部来自本会话附件（AGNT-013），
// 返回按附件顺序映射的 URL 列表。
func resolveAttachments(session *Session, requested []int64, maxImages int) ([]int64, []string, error) {
	if len(requested) == 0 {
		return nil, nil, nil
	}
	if len(requested) > maxImages {
		return nil, nil, errx.New(errx.ParamError, "too many images for a single post")
	}
	byID := make(map[int64]string, len(session.Attachments))
	for _, attachment := range session.Attachments {
		byID[attachment.MediaID] = attachment.URL
	}
	ids := make([]int64, 0, len(requested))
	urls := make([]string, 0, len(requested))
	for _, id := range requested {
		url, ok := byID[id]
		if !ok {
			return nil, nil, errx.New(errx.ParamError,
				"image mediaId is not part of this conversation's attachments")
		}
		ids = append(ids, id)
		urls = append(urls, url)
	}
	return ids, urls, nil
}

func createPostExecutor(content contentservice.ContentService, media mediaservice.MediaService) executorFunc {
	return func(ctx context.Context, session *Session, callID string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args postWriteArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "create_post arguments are invalid")
		}
		title := strings.TrimSpace(args.Title)
		body := strings.TrimSpace(args.Content)
		if title == "" || body == "" {
			return "", nil, errx.New(errx.ParamError, "create_post requires title and content")
		}
		if err := validateTextLimits(title, body, args.Tags); err != nil {
			return "", nil, err
		}
		status := int32(1)
		if args.Status != nil {
			status = *args.Status
		}
		mediaIDs, imageURLs, err := resolveAttachments(session, args.ImageMediaIDs, maxPostImages)
		if err != nil {
			return "", nil, err
		}
		if len(mediaIDs) > 0 && media != nil {
			if err := assertMediaOwnership(ctx, media, session.UserID, mediaIDs); err != nil {
				return "", nil, err
			}
		}
		// 幂等键由服务端从请求标识派生（AGNT-015）：同一轮内同一 callID 的重试
		// 命中同一条幂等记录，不会产生重复帖子。
		idempotencyKey := deriveIdempotencyKey(session.RequestID, callID, "create")
		response, err := content.CreatePost(ctx, &contentservice.CreatePostReq{
			AuthorId: session.UserID, Title: title, Content: body,
			Images: imageURLs, Tags: sanitizeTags(args.Tags), Status: status,
			IdempotencyKey: idempotencyKey, MediaIds: mediaIDs,
		})
		if err != nil {
			logx.WithContext(ctx).Errorw("agent create_post rpc failed", logx.Field("err", err.Error()))
			return "", nil, errx.FromRPCError(err)
		}
		output := fmt.Sprintf("帖子创建成功: post_id=%d status=%d revision=%d。",
			response.PostId, response.Status, response.Revision)
		return output, nil, nil
	}
}

type updateArgs struct {
	postWriteArgs
	PostID           int64 `json:"post_id"`
	ExpectedRevision int64 `json:"expected_revision"`
}

func updatePostExecutor(content contentservice.ContentService, media mediaservice.MediaService) executorFunc {
	return func(ctx context.Context, session *Session, callID string, argsJSON string) (string, []tool.Source, error) {
		var args updateArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "update_post arguments are invalid")
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "update_post requires a positive post_id")
		}
		title := strings.TrimSpace(args.Title)
		body := strings.TrimSpace(args.Content)
		if title == "" && body == "" && args.Tags == nil && args.ImageMediaIDs == nil && args.Status == nil {
			return "", nil, errx.New(errx.ParamError, "update_post requires at least one field to change")
		}
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		if title != "" || body != "" {
			if err := validateTextLimits(title, body, args.Tags); err != nil {
				return "", nil, err
			}
		}
		expectedRevision := args.ExpectedRevision
		if expectedRevision <= 0 {
			// AGNT-014：未提供版本号时先读当前版本再写入。
			current, err := content.GetPost(ctx, &contentservice.GetPostReq{
				PostId: args.PostID, UserId: session.UserID,
			})
			if err != nil {
				logx.WithContext(ctx).Errorw("agent update_post pre-read failed", logx.Field("err", err.Error()))
				return "", nil, errx.FromRPCError(err)
			}
			expectedRevision = current.GetPost().GetRevision()
		}
		var (
			mediaIDs  []int64
			imageURLs []string
			err       error
		)
		if args.ImageMediaIDs != nil {
			mediaIDs, imageURLs, err = resolveAttachments(session, args.ImageMediaIDs, maxPostImages)
			if err != nil {
				return "", nil, err
			}
		}
		if len(mediaIDs) > 0 && media != nil {
			if err := assertMediaOwnership(ctx, media, session.UserID, mediaIDs); err != nil {
				return "", nil, err
			}
		}
		request := &contentservice.UpdatePostReq{
			PostId: args.PostID, AuthorId: session.UserID,
			ExpectedRevision: expectedRevision,
		}
		if title != "" {
			request.Title = title
		}
		if body != "" {
			request.Content = body
		}
		if args.Tags != nil {
			request.Tags = sanitizeTags(args.Tags)
		}
		if args.Status != nil {
			request.Status = args.Status
		}
		if imageURLs != nil {
			request.Images = imageURLs
			request.MediaIds = mediaIDs
		}
		response, err := content.UpdatePost(ctx, request)
		if err != nil {
			logx.WithContext(ctx).Errorw("agent update_post rpc failed", logx.Field("err", err.Error()))
			return "", nil, errx.FromRPCError(err)
		}
		output := fmt.Sprintf("帖子更新成功: post_id=%d revision=%d status=%d。",
			args.PostID, response.Revision, response.Status)
		return output, nil, nil
	}
}

func deletePostExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, callID string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID           int64 `json:"post_id"`
			ExpectedRevision int64 `json:"expected_revision"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "delete_post arguments are invalid")
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "delete_post requires a positive post_id")
		}
		expectedRevision := args.ExpectedRevision
		title := ""
		if expectedRevision <= 0 {
			current, err := content.GetPost(ctx, &contentservice.GetPostReq{
				PostId: args.PostID, UserId: session.UserID,
			})
			if err != nil {
				logx.WithContext(ctx).Errorw("agent delete_post pre-read failed", logx.Field("err", err.Error()))
				return "", nil, errx.FromRPCError(err)
			}
			expectedRevision = current.GetPost().GetRevision()
			title = current.GetPost().GetTitle()
		}

		// AGNT-020~022：删除前逐次确认；拒绝或超时都不执行并把结果回灌给模型。
		if session.Confirms == nil || session.Emit == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		summary := fmt.Sprintf("删除帖子 #%d%s", args.PostID, confirmTitleSuffix(title))
		timeout := time.Duration(session.Budget.ConfirmTimeout) * time.Second
		if err := session.Confirms.Open(ctx, session.UserID, session.RequestID, callID, ToolDeletePost, summary, timeout); err != nil {
			return "", nil, err
		}
		if err := session.Emit(&pb.ChatEvent{
			Type:           pb.ChatEventType_CHAT_EVENT_TYPE_CONFIRM_REQUIRED,
			ConversationId: session.ConversationID,
			ToolCall: &pb.ToolCallInfo{
				CallId: callID, Tool: ToolDeletePost, Summary: summary,
				PayloadJson: argsJSON,
			},
		}); err != nil {
			return "", nil, err
		}
		approved, err := session.Confirms.Wait(ctx, session.UserID, session.RequestID, callID, timeout)
		if errors.Is(err, ErrConfirmExpired) {
			metricAgentConfirmsTotal.Inc("expired")
			return "用户未在时限内确认，删除操作已取消。请告知用户如需删除可以再次发起。", nil, nil
		}
		if err != nil {
			return "", nil, err
		}
		if !approved {
			metricAgentConfirmsTotal.Inc("declined")
			return "用户拒绝了本次删除操作。请不要重复尝试，除非用户明确要求。", nil, nil
		}
		metricAgentConfirmsTotal.Inc("approved")
		if _, err := content.DeletePost(ctx, &contentservice.DeletePostReq{
			PostId: args.PostID, AuthorId: session.UserID, ExpectedRevision: expectedRevision,
		}); err != nil {
			logx.WithContext(ctx).Errorw("agent delete_post rpc failed", logx.Field("err", err.Error()))
			return "", nil, errx.FromRPCError(err)
		}
		return fmt.Sprintf("帖子 #%d 已删除。", args.PostID), nil, nil
	}
}

func confirmTitleSuffix(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if runes := []rune(title); len(runes) > 40 {
		title = string(runes[:40]) + "…"
	}
	return fmt.Sprintf("《%s》", title)
}

func validateTextLimits(title, content string, tags []string) error {
	if title != "" && utf8.RuneCountInString(title) > maxTitleRunes {
		return errx.New(errx.ParamError, "title exceeds 120 characters")
	}
	if content != "" && utf8.RuneCountInString(content) > maxContentRunes {
		return errx.New(errx.ParamError, "content exceeds 20000 characters")
	}
	if len(tags) > maxTags {
		return errx.New(errx.ParamError, fmt.Sprintf("at most %d tags are allowed", maxTags))
	}
	for _, tag := range tags {
		if utf8.RuneCountInString(tag) > maxTagRunes {
			return errx.New(errx.ParamError, "tag exceeds 32 characters")
		}
	}
	return nil
}

// sanitizeTags 规范化标签：去空白、去空项。
func sanitizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		result = append(result, tag)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// assertMediaOwnership 预检媒体归属与状态（AGNT-013）；Content 写路径的
// validatePostMedia 仍会二次校验，这里提前失败以免整次写请求被拒。
func assertMediaOwnership(ctx context.Context, media mediaservice.MediaService, userID int64, mediaIDs []int64) error {
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: mediaIDs})
	if err != nil {
		return errx.FromRPCError(err)
	}
	found := make(map[int64]*mediaservice.MediaInfo, len(response.GetMedias()))
	for _, info := range response.GetMedias() {
		found[info.GetId()] = info
	}
	for _, id := range mediaIDs {
		info := found[id]
		switch {
		case info == nil:
			return errx.New(errx.MediaNotFound, "attached media does not exist")
		case info.GetUserId() != userID:
			return errx.New(errx.PermissionDenied, "attached media belongs to another user")
		case info.GetStatus() != 1:
			return errx.New(errx.ParamError, "attached media is not available")
		}
	}
	return nil
}

func deriveIdempotencyKey(requestID, callID, action string) string {
	key := fmt.Sprintf("agent:%s:%s:%s", action, requestID, callID)
	if len(key) > 128 {
		key = key[:128]
	}
	return key
}

func strictUnmarshal(raw string, target any) error {
	if raw == "" {
		raw = "{}"
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		// 模型可能生成多余字段；宽松降级为标准解析，仍要求顶层是对象。
		return json.Unmarshal([]byte(raw), target)
	}
	return nil
}
