package logic

import (
	"context"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostList 获取帖子列表（keyset 游标分页，无 count(*)）
func (l *GetPostListLogic) GetPostList(in *pb.GetPostListReq) (*pb.GetPostListResp, error) {
	pageSize := normalizePageSize(int(in.PageSize))
	sortBy := normalizeSortBy(int(in.SortBy), true)

	cursor, err := decodePostCursor(in.Cursor, postListGlobal)
	if err != nil {
		return nil, err
	}
	if cursor == nil {
		// 首页也必须携带排序模式，Model 据此选择 keyset 键列。
		cursor = &model.PostListCursor{SortBy: sortBy}
	}

	posts, hasMore, err := l.svcCtx.PostModel.FindListByCursor(l.ctx, cursor, pageSize)
	if err != nil {
		if model.ErrInvalidCursorArity(err) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		l.Errorw("PostModel.FindListByCursor failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostListResp{Posts: []*pb.PostInfo{}}, nil
	}

	var nextCursor string
	if hasMore && len(posts) > 0 {
		boundary := posts[len(posts)-1]
		nextCursor, err = encodePostCursor(sortBy, boundary)
		if err != nil {
			l.Errorw("encode post cursor failed", logx.Field("err", err.Error()))
			nextCursor = ""
		}
	}

	posts = keepPublishedPosts(posts)
	if len(posts) == 0 {
		return &pb.GetPostListResp{Posts: []*pb.PostInfo{}, NextCursor: nextCursor}, nil
	}

	postInfos := hydratePostInfos(l.ctx, l.svcCtx, l.Logger, posts)
	return &pb.GetPostListResp{
		Posts:      postInfos,
		NextCursor: nextCursor,
	}, nil
}
