package logic

import (
	"context"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/pageutil"

	"esx/pkg/logging"
)

type GetTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTagsLogic {
	return &GetTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetTags 获取标签列表（按帖子数降序）
func (l *GetTagsLogic) GetTags(in *pb.GetTagsReq) (*pb.GetTagsResp, error) {
	limit := int(pageutil.ClampPageSizeTo(in.Limit, 20, maxTagListLimit))

	tags, err := l.svcCtx.TagModel.FindList(l.ctx, limit)
	if err != nil {
		l.Errorw("TagModel.FindList failed", logging.Field("err", err.Error()))
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
