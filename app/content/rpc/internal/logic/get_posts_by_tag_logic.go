package logic

import (
	"context"
	"errx"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsByTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostsByTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByTagLogic {
	return &GetPostsByTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostsByTag 获取标签下的帖子
func (l *GetPostsByTagLogic) GetPostsByTag(in *pb.GetPostsByTagReq) (*pb.GetPostsByTagResp, error) {
	if in.TagName == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	page := int(in.Page)
	pageSize := int(in.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	// FindPostIdsByTagName 已 JOIN post 过滤 status=1，total 与数据一致
	postIds, total, err := l.svcCtx.PostTagModel.FindPostIdsByTagName(l.ctx, in.TagName, page, pageSize)
	if err != nil {
		l.Errorw("PostTagModel.FindPostIdsByTagName failed",
			logx.Field("tagName", in.TagName),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 批量加载帖子，避免 N 次 FindOne
	posts, err := l.svcCtx.PostModel.FindByIds(l.ctx, postIds)
	if err != nil {
		l.Errorw("PostModel.FindByIds failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostsByTagResp{Posts: []*pb.PostInfo{}, Total: total}, nil
	}

	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, postIds)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
		tagsMap = map[int64][]string{}
	}
	postInfos := make([]*pb.PostInfo, 0, len(posts))
	for _, post := range posts {
		postInfos = append(postInfos, PostToPostInfo(post, tagsMap[post.Id]))
	}

	return &pb.GetPostsByTagResp{
		Posts: postInfos,
		Total: total,
	}, nil
}
