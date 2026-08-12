// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetPersonalizationPreferenceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 设置个性化偏好
func NewSetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetPersonalizationPreferenceLogic {
	return &SetPersonalizationPreferenceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetPersonalizationPreferenceLogic) SetPersonalizationPreference(req *types.SetPersonalizationPreferenceReq) (resp *types.SetPersonalizationPreferenceResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	_, err = l.svcCtx.UserService.SetPersonalizationPreference(l.ctx, &userservice.SetPersonalizationPreferenceReq{
		UserId:  userID,
		Enabled: req.Enabled,
	})
	if err != nil {
		l.Errorw("UserService.SetPersonalizationPreference RPC failed",
			logx.Field("userId", userID),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	return &types.SetPersonalizationPreferenceResp{}, nil
}
