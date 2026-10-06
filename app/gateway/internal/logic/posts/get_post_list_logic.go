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

	"esx/pkg/logging"
)

// GetPostListLogic 承载 GetPostList 接口的业务逻辑；每个请求新建一个实例。
type GetPostListLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子列表
func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetPostList 返回帖子列表，并为登录用户补充点赞/收藏状态与作者信息。
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

	// 互动状态失败直接报错，作者信息只做软加载：缺失时以空作者降级展示。
	postIDs := make([]int64, 0, len(result.Posts))
	for _, post := range result.Posts {
		if post != nil && post.Id > 0 {
			postIDs = append(postIDs, post.Id)
		}
	}
	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, viewerID, postIDs)
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logging.Field("err", err.Error()))
		return nil, err
	}
	authors := authorx.LoadSoft(l.ctx, l.svcCtx, authorx.PostAuthorIDs(result.Posts))

	return &types.GetPostListResp{
		List:       postmap.Items(result.Posts, liked, favorited, authors),
		NextCursor: result.NextCursor,
	}, nil
}
