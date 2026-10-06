package user

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// GetPersonalizationPreferenceLogic 承载 GetPersonalizationPreference 接口的业务逻辑；每个请求新建一个实例。
type GetPersonalizationPreferenceLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取个性化偏好
func NewGetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPersonalizationPreferenceLogic {
	return &GetPersonalizationPreferenceLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetPersonalizationPreference 返回当前用户是否开启个性化推荐及退出时间。
func (l *GetPersonalizationPreferenceLogic) GetPersonalizationPreference() (resp *types.GetPersonalizationPreferenceResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.UserService.GetPersonalizationPreference(l.ctx, &userservice.GetPersonalizationPreferenceReq{
		UserId: userID,
	})
	if err != nil {
		l.Errorw("UserService.GetPersonalizationPreference RPC failed",
			logging.Field("userId", userID),
			logging.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.GetPersonalizationPreferenceResp{
		Enabled:    result.Enabled,
		OptedOutAt: result.OptedOutAt,
	}, nil
}
