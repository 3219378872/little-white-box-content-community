package logic

import (
	"context"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"

	"esx/pkg/errx"
	"esx/pkg/validator"

	"esx/pkg/logging"
)

type GetPostsByIdsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetPostsByIdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByIdsLogic {
	return &GetPostsByIdsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetPostsByIds 批量按 ID 查询帖子（过滤软删除/未发布）
func (l *GetPostsByIdsLogic) GetPostsByIds(in *pb.GetPostsByIdsReq) (*pb.GetPostsByIdsResp, error) {
	if len(in.PostIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	posts, err := l.svcCtx.PostModel.FindByIds(l.ctx, in.PostIds)
	if err != nil {
		l.Errorw("PostModel.FindByIds failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostsByIdsResp{Posts: []*pb.PostInfo{}}, nil
	}

	published := keepPublishedPosts(posts)
	validIds := make([]int64, 0, len(published))
	for _, post := range published {
		validIds = append(validIds, post.Id)
	}

	tagsMap := map[int64][]string{}
	if !in.SkipTags {
		tagsMap, err = l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, validIds)
		if err != nil {
			l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logging.Field("err", err.Error()))
			tagsMap = map[int64][]string{}
		}
	}

	result := make([]*pb.PostInfo, 0, len(published))
	for _, post := range published {
		result = append(result, PostToPostInfo(post, tagsMap[post.Id]))
	}
	return &pb.GetPostsByIdsResp{Posts: result}, nil
}
