package user

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	logx "esx/pkg/logging"
)

type GetPersonalizationPreferenceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取个性化偏好
func NewGetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPersonalizationPreferenceLogic {
	return &GetPersonalizationPreferenceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

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
			logx.Field("userId", userID),
			logx.Field("err", err.Error()),
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
