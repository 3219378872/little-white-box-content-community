package logic

import (
	"context"
	"errors"
	"errx"
	"user/internal/model"

	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPersonalizationPreferenceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPersonalizationPreferenceLogic {
	return &GetPersonalizationPreferenceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取个性化偏好（REL-023）
func (l *GetPersonalizationPreferenceLogic) GetPersonalizationPreference(in *pb.GetPersonalizationPreferenceReq) (*pb.GetPersonalizationPreferenceResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.Personalization == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	preference, err := l.svcCtx.Personalization.Get(l.ctx, in.UserId)
	if err != nil {
		if errors.Is(err, model.ErrPersonalizationPreferenceNotFound) {
			// 默认开启个性化
			return &pb.GetPersonalizationPreferenceResp{Enabled: true}, nil
		}
		l.Errorw("Personalization.Get failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.GetPersonalizationPreferenceResp{
		Enabled:    preference.Enabled,
		OptedOutAt: preference.OptedOutAt.Int64,
	}, nil
}
