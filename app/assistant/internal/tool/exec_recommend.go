package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/pkg/errx"
)

func recommendPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PageSize int32 `json:"page_size"`
		}
		_ = strictUnmarshal(argsJSON, &args)
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = 10
		}
		userID, requestID := int64(0), ""
		if session != nil {
			userID, requestID = session.UserID, session.RequestID
		}
		resp, err := clients.Recommend.GetRecommendPosts(ctx, &recommendservice.GetRecommendPostsReq{
			UserId: userID, Scene: "agent", RequestId: requestID, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(resp.GetPosts()))
		for _, p := range resp.GetPosts() {
			if p != nil && p.PostId > 0 {
				ids = append(ids, p.PostId)
			}
		}
		published, err := publishedPosts(ctx, clients.Content, ids)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0)
		for _, id := range ids {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
			if len(infos) >= 5 {
				break
			}
		}
		text, sources := formatPosts("推荐结果：\n", infos)
		return text, sources, nil
	}
}

func similarPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID int64 `json:"post_id"`
			Limit  int32 `json:"limit"`
		}
		_ = strictUnmarshal(argsJSON, &args)
		if args.PostID <= 0 && session != nil {
			args.PostID = session.ContextPostID
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "similar_posts requires post_id")
		}
		limit := args.Limit
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		requestID := ""
		if session != nil {
			requestID = session.RequestID
		}
		resp, err := clients.Recommend.GetSimilarPosts(ctx, &recommendservice.GetSimilarPostsReq{PostId: args.PostID, Limit: limit, Scene: "agent", RequestId: requestID})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0)
		for _, p := range resp.GetPosts() {
			if p != nil && p.PostId > 0 {
				ids = append(ids, p.PostId)
			}
		}
		published, err := publishedPosts(ctx, clients.Content, ids)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0)
		for _, id := range ids {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
		}
		text, sources := formatPosts("相似帖子：\n", infos)
		return text, sources, nil
	}
}
