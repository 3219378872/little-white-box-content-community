package errx

// 业务错误码定义
const (
	// 通用错误码 1-999
	SUCCESS         = 0
	UNKNOWN_ERROR   = 1
	PARAM_ERROR     = 2
	SYSTEM_ERROR    = 3
	NOT_FOUND       = 4
	TOO_MANY_REQ    = 5
	SERVICE_UNAVAIL = 6

	// 用户相关错误码 1000-1999
	USER_NOT_FOUND      = 1001
	USER_ALREADY_EXIST  = 1002
	PASSWORD_ERROR      = 1003
	TOKEN_EXPIRED       = 1004
	TOKEN_INVALID       = 1005
	LOGIN_REQUIRED      = 1006
	PERMISSION_DENIED   = 1007
	VERIFY_CODE_ERROR   = 1008
	VERIFY_CODE_EXPIRED = 1009

	// 内容相关错误码 2000-2999
	CONTENT_NOT_FOUND    = 2001
	CONTENT_FORBIDDEN    = 2002
	CONTENT_TOO_LONG     = 2003
	CONTENT_EMPTY        = 2004
	TITLE_EMPTY          = 2005
	POST_ALREADY_DELETED = 2006

	// 互动相关错误码 3000-3999
	ALREADY_LIKED      = 3001
	ALREADY_FAVORITED  = 3002
	NOT_LIKED_YET      = 3003
	NOT_FAVORITED_YET  = 3004
	CANNOT_LIKE_SELF   = 3005
	CANNOT_FOLLOW_SELF = 3006

	// 媒体相关错误码 4000-4999
	FILE_TOO_LARGE        = 4001
	FILE_TYPE_NOT_ALLOWED = 4002
	UPLOAD_FAILED         = 4003

	// 搜索相关错误码 5000-5999
	SEARCH_EMPTY   = 5001
	SEARCH_TIMEOUT = 5002
)

// 错误码消息映射
var codeMsg = map[int]string{
	SUCCESS:         "成功",
	UNKNOWN_ERROR:   "未知错误",
	PARAM_ERROR:     "参数错误",
	SYSTEM_ERROR:    "系统错误",
	NOT_FOUND:       "资源不存在",
	TOO_MANY_REQ:    "请求过于频繁",
	SERVICE_UNAVAIL: "服务不可用",

	USER_NOT_FOUND:      "用户不存在",
	USER_ALREADY_EXIST:  "用户已存在",
	PASSWORD_ERROR:      "密码错误",
	TOKEN_EXPIRED:       "Token已过期",
	TOKEN_INVALID:       "Token无效",
	LOGIN_REQUIRED:      "请先登录",
	PERMISSION_DENIED:   "权限不足",
	VERIFY_CODE_ERROR:   "验证码错误",
	VERIFY_CODE_EXPIRED: "验证码已过期",

	CONTENT_NOT_FOUND:    "内容不存在",
	CONTENT_FORBIDDEN:    "无权操作此内容",
	CONTENT_TOO_LONG:     "内容过长",
	CONTENT_EMPTY:        "内容不能为空",
	TITLE_EMPTY:          "标题不能为空",
	POST_ALREADY_DELETED: "帖子已删除",

	ALREADY_LIKED:      "已点赞",
	ALREADY_FAVORITED:  "已收藏",
	NOT_LIKED_YET:      "未点赞",
	NOT_FAVORITED_YET:  "未收藏",
	CANNOT_LIKE_SELF:   "不能点赞自己",
	CANNOT_FOLLOW_SELF: "不能关注自己",

	FILE_TOO_LARGE:        "文件过大",
	FILE_TYPE_NOT_ALLOWED: "文件类型不支持",
	UPLOAD_FAILED:         "上传失败",

	SEARCH_EMPTY:   "搜索关键词为空",
	SEARCH_TIMEOUT: "搜索超时",
}

// GetMsg 获取错误码对应的消息
func GetMsg(code int) string {
	if msg, ok := codeMsg[code]; ok {
		return msg
	}
	return "未知错误"
}
