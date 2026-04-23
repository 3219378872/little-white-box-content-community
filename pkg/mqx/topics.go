package mqx

// Topic 定义
const (
	// 用户相关 Topic
	TopicUserRegister = "user-register" // 用户注册事件
	TopicUserFollow   = "user-follow"   // 用户关注事件
	TopicUserUnfollow = "user-unfollow" // 用户取关事件

	// 内容相关 Topic
	TopicPostCreate    = "post-create"    // 帖子创建事件
	TopicPostUpdate    = "post-update"    // 帖子更新事件
	TopicPostDelete    = "post-delete"    // 帖子删除事件
	TopicCommentCreate = "comment-create" // 评论创建事件
	TopicCommentDelete = "comment-delete" // 评论删除事件

	// 互动相关 Topic
	TopicLike       = "like"       // 点赞事件
	TopicUnlike     = "unlike"     // 取消点赞事件
	TopicFavorite   = "favorite"   // 收藏事件
	TopicUnfavorite = "unfavorite" // 取消收藏事件

	// 搜索相关 Topic
	TopicSearchIndex  = "search-index"  // 搜索索引事件
	TopicSearchDelete = "search-delete" // 搜索删除事件

	// 推荐相关 Topic
	TopicUserBehavior = "user-behavior" // 用户行为事件

	// Feed 相关 Topic
	TopicFeedGenerate = "feed-generate" // Feed 生成事件

	// 消息相关 Topic
	TopicMessagePush = "message-push" // 消息推送事件

	// 媒体相关 Topic
	TopicMediaDelete = "media-deleted" // 媒体删除事件（触发 S3 清理）
)

// Tag 定义
const (
	TagDefault = "default"
)

// ConsumerGroup 消费者组定义
const (
	GroupUserService      = "user-service-group"
	GroupContentService   = "content-service-group"
	GroupSearchService    = "search-service-group"
	GroupFeedService      = "feed-service-group"
	GroupRecommendService = "recommend-service-group"
	GroupMediaService     = "media-service-group"
)
