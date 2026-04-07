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
	ContentNotFound    = 2001
	ContentForbidden   = 2002
	ContentTooLong     = 2003
	ContentEmpty       = 2004
	TitleEmpty         = 2005
	PostAlreadyDeleted = 2006

	// 互动相关错误码 3000-3999
	AlreadyLiked     = 3001
	AlreadyFavorited = 3002
	NotLikedYet      = 3003
	NotFavoritedYet  = 3004
	CannotLikeSelf   = 3005
	CannotFollowSelf = 3006

	// 媒体相关错误码 4000-4999
	FileTooLarge       = 4001
	FileTypeNotAllowed = 4002
	UploadFailed       = 4003

	// 搜索相关错误码 5000-5999
	SearchEmpty   = 5001
	SearchTimeout = 5002
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

	ContentNotFound:    "内容不存在",
	ContentForbidden:   "无权操作此内容",
	ContentTooLong:     "内容过长",
	ContentEmpty:       "内容不能为空",
	TitleEmpty:         "标题不能为空",
	PostAlreadyDeleted: "帖子已删除",

	AlreadyLiked:     "已点赞",
	AlreadyFavorited: "已收藏",
	NotLikedYet:      "未点赞",
	NotFavoritedYet:  "未收藏",
	CannotLikeSelf:   "不能点赞自己",
	CannotFollowSelf: "不能关注自己",

	FileTooLarge:       "文件过大",
	FileTypeNotAllowed: "文件类型不支持",
	UploadFailed:       "上传失败",

	SearchEmpty:   "搜索关键词为空",
	SearchTimeout: "搜索超时",
}

// GetMsg 获取错误码对应的消息
func GetMsg(code int) string {
	if msg, ok := codeMsg[code]; ok {
		return msg
	}
	return "未知错误"
}
