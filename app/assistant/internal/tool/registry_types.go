package tool

import (
	"context"
	"esx/app/assistant/internal/consent"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/websearch"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
)

const (
	// 工具名，即模型调用时使用的函数名。
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
	WebSearch       = "web_search"
	CreatePost      = "create_post"
	UpdatePost      = "update_post"
	DeletePost      = "delete_post"
	SearchHistory   = "search_history"
	PresentSources  = "present_sources"
	AskQuestions    = "ask_questions"
	ReadSource      = "read_source"
	PublishAnswer   = "publish_answer"

	// 当前授权版本，以及工具结果与写入内容的上限。
	CurrentConsentVersion   int32 = consent.CurrentVersion
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
	// 工具副作用类别与幂等策略。
	EffectRead  = "read"
	EffectWrite = "write"

	IdempotencyNone    = "none"
	IdempotencyRequest = "request"
)

// Version1Tools 是授权版本 1 即可使用的工具。
func Version1Tools() []string {
	return []string{SearchPosts, WebSearch, CreatePost, UpdatePost, DeletePost}
}

// ReviewTools 是后台记忆回顾可用的工具。
func ReviewTools() []string {
	return []string{GetMemory, AddMemory, ReplaceMemory, RemoveMemory, BatchMemory}
}

// Clients 是工具执行依赖的下游服务；未配置的依赖会使相关工具不可用。
type Clients struct {
	Search      searchservice.SearchService
	Content     contentservice.ContentService
	Media       mediaservice.MediaService
	Recommend   recommendservice.RecommendService
	Interaction interactionservice.InteractionService
	User        userservice.UserService
	Web         websearch.Searcher
	Memory      memory.Store
	Store       store.Store
	History     History
}

// Attachment 是本次对话中用户上传的媒体，发帖工具只能引用这些图片。
type Attachment struct {
	MediaID int64
	URL     string
}

// Session 是一次工具调用的上下文：调用者身份、run 与租约栅栏、授权版本及对话附件。
type Session struct {
	ClientProtocolVersion int
	Question              *store.QuestionRequest
	Answer                *store.AnswerPresentation
	UserID                int64
	SessionID             int64
	RunID                 int64
	RequestID             string
	Source                string
	ConsentVersion        int32
	Attachments           []Attachment
	ContextPostID         int64
	LiveMessageIDs        []int64
	ChangeIDs             []int64
	Fence                 store.LeaseFence
	Recovery              bool
}

// History 检索当前上下文之外的助手历史。
type History interface {
	Search(ctx context.Context, sess *Session, args HistoryArgs) (string, error)
}

// HistoryArgs 是 search_history 的参数；Shape 决定检索方式。
type HistoryArgs struct {
	Shape     string `json:"shape"`
	Query     string `json:"query"`
	MessageID int64  `json:"message_id"`
	SessionID int64  `json:"session_id"`
	Limit     int    `json:"limit"`
}

// Definition 是一个工具的完整定义，含执行器与可选的参数预处理器。
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Metadata    Metadata
	executor    executorFunc
	prepare     prepareFunc
}

// Metadata 是工具的策略属性：副作用、可用来源、所需授权与协议版本、是否需确认及结果上限。
type Metadata struct {
	MinClientProtocol int
	Effect            string
	Sources           []string
	MinConsent        int32
	Confirmation      bool
	Available         bool
	Idempotency       string
	MaxResultBytes    int
	Poller            bool
}

// UnavailableError 表示工具依赖未配置；调用方据此返回结构化的不可用结果。
type UnavailableError struct {
	Tool string
}

// Error 不暴露内部依赖信息。
func (e *UnavailableError) Error() string { return "agent tool is unavailable" }

// executorFunc 执行一次工具调用，返回给模型的文本与待登记的来源。
type executorFunc func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error)

// prepareFunc 在执行前补全或校验参数，结果参与幂等摘要与确认。
type prepareFunc func(ctx context.Context, session *Session, argsJSON string) (string, error)

// Registry 是可用工具的集合；frozen 时以会话冻结的定义为准，保证同一会话内工具集稳定。
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
