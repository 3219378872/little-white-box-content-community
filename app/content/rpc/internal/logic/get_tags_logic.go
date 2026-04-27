package logic

import (
	"context"
	"errx"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTagsLogic {
	return &GetTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetTags 获取标签列表（按帖子数降序）
func (l *GetTagsLogic) GetTags(in *pb.GetTagsReq) (*pb.GetTagsResp, error) {
	limit := int(in.Limit)
	if limit <= 0 {
		limit = 20
	}

	tags, err := l.svcCtx.TagModel.FindList(l.ctx, limit)
	if err != nil {
		l.Errorw("TagModel.FindList failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	tagInfos := make([]*pb.TagInfo, 0, len(tags))
	for _, t := range tags {
		tagInfos = append(tagInfos, TagToTagInfo(t))
	}

	return &pb.GetTagsResp{
		Tags: tagInfos,
	}, nil
}
