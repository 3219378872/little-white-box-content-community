package logic

import (
	"context"
	"esx/pkg/errx"

	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"

	"esx/pkg/logging"
)

type GetUserTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetUserTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserTagsLogic {
	return &GetUserTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 获取用户标签
func (l *GetUserTagsLogic) GetUserTags(in *pb.GetUserTagsReq) (*pb.GetUserTagsResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.UserTagModel == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	tags, err := l.svcCtx.UserTagModel.FindByUserId(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserTagModel.FindByUserId failed", logging.Field("user_id", in.UserId), logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != nil && tag.TagName != "" {
			result = append(result, tag.TagName)
		}
	}
	return &pb.GetUserTagsResp{Tags: result}, nil
}
