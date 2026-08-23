package logic

import (
	"context"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPostsLogic {
	return &GetUserPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserPosts 获取用户帖子列表（keyset 游标分页，无 count(*)）
func (l *GetUserPostsLogic) GetUserPosts(in *pb.GetUserPostsReq) (*pb.GetUserPostsResp, error) {
	pageSize := normalizePageSize(int(in.PageSize))
	sortBy := normalizeSortBy(int(in.SortBy), false)

	cursor, err := decodePostCursor(in.Cursor, postListUserPosts)
	if err != nil {
		return nil, err
	}
	if cursor == nil {
		cursor = &model.PostListCursor{SortBy: sortBy}
	}

	posts, hasMore, err := l.svcCtx.PostModel.FindUserPostsByCursor(l.ctx, in.UserId, cursor, pageSize)
	if err != nil {
		if model.ErrInvalidCursorArity(err) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		l.Errorw("PostModel.FindUserPostsByCursor failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetUserPostsResp{Posts: []*pb.PostInfo{}}, nil
	}

	var nextCursor string
	if hasMore && len(posts) > 0 {
		boundary := posts[len(posts)-1]
		nextCursor, err = encodePostCursor(sortBy, boundary)
		if err != nil {
			l.Errorw("encode user posts cursor failed", logx.Field("err", err.Error()))
			nextCursor = ""
		}
	}

	posts = keepPublishedPosts(posts)
	if len(posts) == 0 {
		return &pb.GetUserPostsResp{Posts: []*pb.PostInfo{}, NextCursor: nextCursor}, nil
	}

	postInfos := hydratePostInfos(l.ctx, l.svcCtx, l.Logger, posts)
	return &pb.GetUserPostsResp{
		Posts:      postInfos,
		NextCursor: nextCursor,
	}, nil
}

// hydratePostInfos 批量补齐标签并转换为 pb 结构（列表类 logic 共用）。
func hydratePostInfos(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	logger logx.Logger,
	posts []*model.Post,
) []*pb.PostInfo {
	postIds := make([]int64, 0, len(posts))
	for _, post := range posts {
		postIds = append(postIds, post.Id)
	}
	tagsMap, err := svcCtx.PostTagModel.FindTagNamesByPostIds(ctx, postIds)
	if err != nil {
		logger.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
		tagsMap = map[int64][]string{}
	}
	postInfos := make([]*pb.PostInfo, 0, len(posts))
	for _, post := range posts {
		postInfos = append(postInfos, PostToPostInfo(post, tagsMap[post.Id]))
	}
	return postInfos
}
