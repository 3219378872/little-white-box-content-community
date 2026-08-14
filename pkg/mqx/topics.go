package mqx

// Topic 定义。本清单与 deploy/rocketmq/init-topics.sh 保持一致；
// 新增或退役主题必须同步两侧（见 deploy/rocketmq_topics_test.go）。
const (
	// 内容相关 Topic
	TopicPostCreate = "post-create" // 帖子创建事件
	TopicPostUpdate = "post-update" // 帖子更新事件
	TopicPostDelete = "post-delete" // 帖子删除事件

	// 推荐相关 Topic
	TopicUserBehaviorV2 = "user-behavior-v2" // 统一行为事件

	// 消息相关 Topic
	TopicMessagePush = "message-push" // 消息推送事件

	// 媒体相关 Topic
	TopicMediaDelete = "media-deleted" // 媒体删除事件（触发 S3 清理）
)

// Tag 定义
const (
	TagDefault = "default"
)

// ConsumerGroup 定义。行为日志管道消费者组由配置引用，见
// app/pipeline/behaviorlog/etc/behavior-log.yaml。
const (
	GroupBehaviorLogService = "behavior-log-service-group"
)
