package cachex

// 缓存 key 前缀定义
const (
	// 用户相关缓存
	UserInfoPrefix      = "user:info:"      // 用户信息
	UserProfilePrefix   = "user:profile:"   // 用户详情
	UserFollowPrefix    = "user:follow:"    // 关注关系
	UserFollowerPrefix  = "user:follower:"  // 粉丝列表
	UserFollowingPrefix = "user:following:" // 关注列表
	UserStatsPrefix     = "user:stats:"     // 用户统计

	// 内容相关缓存
	PostInfoPrefix    = "post:info:"    // 帖子信息
	PostListPrefix    = "post:list:"    // 帖子列表
	PostCommentPrefix = "post:comment:" // 帖子评论
	PostHotPrefix     = "post:hot:"     // 热门帖子
	PostTagsPrefix    = "post:tags:"    // 帖子标签

	// 互动相关缓存
	LikeCountPrefix    = "like:count:"    // 点赞数
	LikeUserPrefix     = "like:user:"     // 用户点赞记录
	CommentCountPrefix = "comment:count:" // 评论数
	FavoritePrefix     = "favorite:"      // 收藏

	// 媒体相关缓存
	MediaInfoPrefix = "media:info:" // 媒体信息
	MediaUrlPrefix  = "media:url:"  // 媒体URL

	// 搜索相关缓存
	SearchHotPrefix     = "search:hot:"     // 热门搜索
	SearchHistoryPrefix = "search:history:" // 搜索历史

	// 验证码缓存
	VerifyCodePrefix   = "verify:code:"     // 验证码
	VerifyCodeCooldown = "verify:cooldown:" // 验证码冷却

	// 会话缓存
	SessionPrefix = "session:" // 会话
	TokenPrefix   = "token:"   // Token

	// 限流缓存
	RateLimitPrefix = "rate:limit:" // 限流
)

// BuildKey 构建缓存 key
func BuildKey(prefix string, suffix ...interface{}) string {
	key := prefix
	for _, s := range suffix {
		key += toString(s)
	}
	return key
}

// toString 将任意类型转换为字符串
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return intToStr(int64(val))
	case int64:
		return intToStr(val)
	case int32:
		return intToStr(int64(val))
	case uint:
		return intToStr(int64(val))
	case uint64:
		return intToStr(int64(val))
	case uint32:
		return intToStr(int64(val))
	default:
		return ""
	}
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	var negative bool
	if n < 0 {
		negative = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
