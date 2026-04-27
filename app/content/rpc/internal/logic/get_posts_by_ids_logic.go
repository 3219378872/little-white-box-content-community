package logic

import (
	"context"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"errx"
	"esx/pkg/validator"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsByIdsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostsByIdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByIdsLogic {
	return &GetPostsByIdsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostsByIds 批量按 ID 查询帖子（过滤软删除/未发布）
func (l *GetPostsByIdsLogic) GetPostsByIds(in *pb.GetPostsByIdsReq) (*pb.GetPostsByIdsResp, error) {
	if len(in.PostIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	posts, err := l.svcCtx.PostModel.FindByIds(l.ctx, in.PostIds)
	if err != nil {
		l.Errorw("PostModel.FindByIds failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostsByIdsResp{Posts: []*pb.PostInfo{}}, nil
	}

	validIds := make([]int64, 0, len(posts))
	for _, p := range posts {
		if p.Status == 1 {
			validIds = append(validIds, p.Id)
		}
	}

	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, validIds)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
		tagsMap = map[int64][]string{}
	}

	result := make([]*pb.PostInfo, 0, len(validIds))
	for _, p := range posts {
		if p.Status != 1 {
			continue
		}
		result = append(result, PostToPostInfo(p, tagsMap[p.Id]))
	}
	return &pb.GetPostsByIdsResp{Posts: result}, nil
}
