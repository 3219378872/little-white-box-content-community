package errx

// 业务错误码定义
const (
	// 通用错误码 1-999
	SUCCESS            = 0
	UnknownError       = 1
	ParamError         = 2
	SystemError        = 3
	NotFound           = 4
	TooManyReq         = 5
	ServiceUnavailable = 6

	// 用户相关错误码 1000-1999
	UserNotFound      = 1001
	UserAlreadyExist  = 1002
	PasswordError     = 1003
	TokenExpired      = 1004
	TokenInvalid      = 1005
	LoginRequired     = 1006
	PermissionDenied  = 1007
	VerifyCodeError   = 1008
	VerifyCodeExpired = 1009

	// 内容相关错误码 2000-2999
	ContentNotFound        = 2001
	ContentForbidden       = 2002
	ContentTooLong         = 2003
	ContentEmpty           = 2004
	TitleEmpty             = 2005
	PostAlreadyDeleted     = 2006
	ContentVersionConflict = 2007
	IdempotencyConflict    = 2008

	// 互动相关错误码 3000-3999
	CannotFollowSelf = 3006
	FavoritesPrivate = 3007

	// 媒体相关错误码 4000-4999
	FileTooLarge       = 4001
	FileTypeNotAllowed = 4002
	UploadFailed       = 4003
	MediaNotFound      = 4004
	MediaMetaMissing   = 4005
	MediaProcessFailed = 4006

	// 搜索相关错误码 5000-5999
	SearchEmpty   = 5001
	SearchTimeout = 5002

	// Assistant Agent 相关错误码 6000-6999（SPEC-assistant-agent）
	AgentNotAuthorized = 6001
	AgentResourceLimit = 6002
	AgentQueueFull     = 6003
	AgentRunConflict   = 6004
	// 6005 曾为 CannotWatchSelf，随 Watch 退役后保留不复用。

	// 审核平台与付费广告错误码 7000-7999（SPEC-review-platform、SPEC-sponsored-ads）
	ReviewLeaseLost         = 7001
	ReviewTaskSuperseded    = 7002
	ReviewRoleRequired      = 7003
	ReviewTaskDecided       = 7004
	AdvertiserRequired      = 7101
	AdvertiserExists        = 7102
	AdQualificationRequired = 7103
	AdLandingInvalid        = 7104
	AdIndustryUnsupported   = 7105
	AdAppealNotAllowed      = 7106
	AdMediaInvalid          = 7107
)

// 错误码消息映射
var codeMsg = map[int]string{
	SUCCESS:            "成功",
	UnknownError:       "未知错误",
	ParamError:         "参数错误",
	SystemError:        "系统错误",
	NotFound:           "资源不存在",
	TooManyReq:         "请求过于频繁",
	ServiceUnavailable: "服务不可用",

	UserNotFound:      "用户不存在",
	UserAlreadyExist:  "用户已存在",
	PasswordError:     "密码错误",
	TokenExpired:      "Token已过期",
	TokenInvalid:      "Token无效",
	LoginRequired:     "请先登录",
	PermissionDenied:  "权限不足",
	VerifyCodeError:   "验证码错误",
	VerifyCodeExpired: "验证码已过期",

	ContentNotFound:        "内容不存在",
	ContentForbidden:       "无权操作此内容",
	ContentTooLong:         "内容过长",
	ContentEmpty:           "内容不能为空",
	TitleEmpty:             "标题不能为空",
	PostAlreadyDeleted:     "帖子已删除",
	ContentVersionConflict: "内容版本冲突",
	IdempotencyConflict:    "幂等键已用于其他命令",

	CannotFollowSelf: "不能关注自己",
	FavoritesPrivate: "收藏列表已设为私密",

	FileTooLarge:       "文件过大",
	FileTypeNotAllowed: "文件类型不支持",
	UploadFailed:       "上传失败",
	MediaNotFound:      "媒体不存在或已删除",
	MediaMetaMissing:   "上传元数据缺失",
	MediaProcessFailed: "媒体处理失败",

	SearchEmpty:   "搜索关键词为空",
	SearchTimeout: "搜索超时",

	AgentNotAuthorized: "Agent 能力未授权",
	AgentResourceLimit: "Agent 资源预算已耗尽",
	AgentQueueFull:     "Agent 输入队列已满",
	AgentRunConflict:   "Agent 运行状态冲突",

	ReviewLeaseLost:         "审核任务持有已失效",
	ReviewTaskSuperseded:    "审核任务已作废",
	ReviewRoleRequired:      "缺少审核权限",
	ReviewTaskDecided:       "审核任务已有结论",
	AdvertiserRequired:      "需要已过审的广告主身份",
	AdvertiserExists:        "广告主已存在",
	AdQualificationRequired: "缺少目标市场要求的行业资质（INDUSTRY.QUALIFICATION）",
	AdLandingInvalid:        "落地页地址不合规（LANDING.URL）",
	AdIndustryUnsupported:   "不支持的行业或市场",
	AdAppealNotAllowed:      "当前版本不可申诉",
	AdMediaInvalid:          "素材不可用",
}

// GetMsg 获取错误码对应的消息
func GetMsg(code int) string {
	if msg, ok := codeMsg[code]; ok {
		return msg
	}
	return "未知错误"
}
