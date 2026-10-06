// Package personalization 声明 recommend-mq 与 recommend-rpc 共用的个性化开关依赖与退出判定：
// 两侧都以 user 服务的持久偏好为准复核 opt-out（REL-023），规则只在这里维护一份。
package personalization

import (
	"context"
	"fmt"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/recommend/featurekey"
	"esx/app/user/rpc/userservice"
)

// PreferenceReader 是读取用户个性化开关的最小依赖，user 服务客户端满足它。
type PreferenceReader interface {
	GetPersonalizationPreference(context.Context, *userservice.GetPersonalizationPreferenceReq, ...callopt.Option) (*userservice.GetPersonalizationPreferenceResp, error)
}

// MarkerGetter 读取 user 服务关闭个性化时写入的退出标记（featurekey.OptOutKey）。
type MarkerGetter interface {
	GetCtx(ctx context.Context, key string) (string, error)
}

// OptedOut 判断登录用户是否关闭了个性化；非登录用户（userID<=0）没有可复核的偏好，按未关闭返回。
// 退出标记存在即视为关闭；标记缺失、过期或缓存不可用都不能证明同意，必须回查 user 服务。
// marker 可为 nil（存储不支持读取标记时）；回查失败一律按已关闭处理并返回错误（fail closed）。
func OptedOut(ctx context.Context, marker MarkerGetter, reader PreferenceReader, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if marker != nil {
		value, err := marker.GetCtx(ctx, featurekey.OptOutKey(userID))
		if err == nil && value != "" {
			return true, nil
		}
	}
	if reader == nil {
		return true, fmt.Errorf("personalization preference service unavailable")
	}
	preference, err := reader.GetPersonalizationPreference(ctx, &userservice.GetPersonalizationPreferenceReq{UserId: userID})
	if err != nil {
		return true, fmt.Errorf("load personalization preference: %w", err)
	}
	if preference == nil {
		return true, fmt.Errorf("personalization preference response is nil")
	}
	return !preference.Enabled, nil
}
