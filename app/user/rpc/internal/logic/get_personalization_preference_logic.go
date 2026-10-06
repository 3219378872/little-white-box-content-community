package logic

import (
	"context"
	"errors"
	"esx/app/user/rpc/internal/model"
	"esx/pkg/errx"

	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"

	"esx/pkg/logging"
)

// GetPersonalizationPreferenceLogic 承载 GetPersonalizationPreference 接口的业务逻辑；每个请求新建一个实例。
type GetPersonalizationPreferenceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetPersonalizationPreferenceLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPersonalizationPreferenceLogic {
	return &GetPersonalizationPreferenceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
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
		l.Errorw("Personalization.Get failed", logging.Field("user_id", in.UserId), logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.GetPersonalizationPreferenceResp{
		Enabled:    preference.Enabled,
		OptedOutAt: preference.OptedOutAt.Int64,
	}, nil
}
