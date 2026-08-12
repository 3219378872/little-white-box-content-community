package logic

import (
	"context"
	"errx"

	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserTagsLogic {
	return &GetUserTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
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
		l.Errorw("UserTagModel.FindByUserId failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
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
