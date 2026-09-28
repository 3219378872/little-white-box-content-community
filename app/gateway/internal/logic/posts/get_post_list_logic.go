package posts

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/logic/postmap"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/logic/viewerstate"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/jwtx"
	"esx/pkg/pageutil"

	logx "esx/pkg/logging"
)

type GetPostListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子列表
func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostListLogic) GetPostList(req *types.GetPostListReq) (resp *types.GetPostListResp, err error) {
	pageSize := pageutil.ClampPageSize(req.PageSize)
	rpcReq := &contentservice.GetPostListReq{
		PageSize: pageSize,
		SortBy:   req.SortBy,
		Cursor:   req.Cursor,
	}
	viewerID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	if viewerID > 0 {
		rpcReq.UserId = viewerID
	}

	result, err := l.svcCtx.ContentService.GetPostList(l.ctx, rpcReq)
	if err != nil {
		return nil, rpcx.Error(l.Logger, "ContentService.GetPostList", err)
	}

	postIDs := make([]int64, 0, len(result.Posts))
	for _, post := range result.Posts {
		if post != nil && post.Id > 0 {
			postIDs = append(postIDs, post.Id)
		}
	}
	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, viewerID, postIDs)
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("err", err.Error()))
		return nil, err
	}
	authors := authorx.LoadSoft(l.ctx, l.svcCtx, authorx.PostAuthorIDs(result.Posts))

	return &types.GetPostListResp{
		List:       postmap.Items(result.Posts, liked, favorited, authors),
		NextCursor: result.NextCursor,
	}, nil
}
