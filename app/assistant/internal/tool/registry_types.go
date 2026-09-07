package tool

import (
	"context"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/websearch"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
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
	AskQuestions    = "ask_questions"
	ReadSource      = "read_source"
	PublishAnswer   = "publish_answer"

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
	return []string{SearchPosts, SearchUsers, SearchTags, GetPost, GetPostComments, RecommendPosts, SimilarPosts, ComparePosts, GetMemory, SearchHistory, PresentSources, ReadSource, PublishAnswer}
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
	WatchPostIDs          []int64
	LiveMessageIDs        []int64
	ChangeIDs             []int64
	Fence                 store.LeaseFence
	Recovery              bool
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
