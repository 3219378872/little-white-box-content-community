// Package personalization 声明 recommend-mq 与 recommend-rpc 共用的个性化开关依赖：
// 两侧都以 user 服务的持久偏好为准复核 opt-out（REL-023），接口只在这里定义一份。
package personalization

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/user/rpc/userservice"
)

// PreferenceReader 是读取用户个性化开关的最小依赖，user 服务客户端满足它。
type PreferenceReader interface {
	GetPersonalizationPreference(context.Context, *userservice.GetPersonalizationPreferenceReq, ...callopt.Option) (*userservice.GetPersonalizationPreferenceResp, error)
}
