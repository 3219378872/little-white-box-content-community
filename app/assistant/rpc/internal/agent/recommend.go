package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	ToolRecommendPosts = "recommend_posts"
	ToolSimilarPosts   = "similar_posts"
	ToolComparePosts   = "compare_posts"
)

func recommendPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PageSize int32 `json:"page_size"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "recommend_posts arguments are invalid")
		}
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = 10
		}
		userID := int64(0)
		requestID := ""
		if session != nil {
			userID = session.UserID
			requestID = session.RequestID
		}
		response, err := clients.Recommend.GetRecommendPosts(ctx, &recommendservice.GetRecommendPostsReq{
			UserId: userID, Scene: "agent", RequestId: requestID, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(response.GetPosts()))
		meta := make(map[int64]*recommendservice.RecommendPost, len(response.GetPosts()))
		for _, post := range response.GetPosts() {
			if post == nil || post.PostId <= 0 {
				continue
			}
			ids = append(ids, post.PostId)
			meta[post.PostId] = post
		}
		excluded := excludedPostIDs(ctx, clients.Memory, userID)
		filtered := ids[:0]
		for _, id := range ids {
			if _, skip := excluded[id]; skip {
				continue
			}
			filtered = append(filtered, id)
		}
		return formatRecommended(ctx, clients.Content, filtered, meta)
	}
}

func similarPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID int64 `json:"post_id"`
			Limit  int32 `json:"limit"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "similar_posts arguments are invalid")
		}
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
		ids := similarPostIDs(ctx, clients.Recommend, args.PostID, limit, session)
		if requestID == "" {
			_ = requestID
		}
		return formatRecommended(ctx, clients.Content, ids, nil)
	}
}

func comparePostsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostIDs []int64 `json:"post_ids"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "compare_posts arguments are invalid")
		}
		if len(args.PostIDs) < 2 || len(args.PostIDs) > 5 {
			return "", nil, errx.New(errx.ParamError, "compare_posts requires 2 to 5 post_ids")
		}
		published, err := tool.PublishedPosts(ctx, content, args.PostIDs)
		if err != nil {
			return "", nil, err
		}
		var text strings.Builder
		sources := make([]tool.Source, 0, len(args.PostIDs))
		for _, id := range args.PostIDs {
			info := published[id]
			if info == nil {
				fmt.Fprintf(&text, "- post:%d 不可见或未发布，已跳过。\n", id)
				continue
			}
			snippet := truncateRunes(info.Content, 160)
			fmt.Fprintf(&text, "- [post:%d] %s\n  %s\n", info.Id, info.Title, snippet)
			sources = append(sources, tool.Source{
				Type: "post", ID: strconv.FormatInt(info.Id, 10), Title: info.Title,
				Snippet: snippet, Revision: info.Revision,
			})
		}
		if len(sources) < 2 {
			return "可比较的已发布帖子不足两篇。", sources, nil
		}
		return strings.TrimRight(text.String(), "\n"), sources, nil
	}
}

func formatRecommended(ctx context.Context, content contentservice.ContentService, ids []int64, meta map[int64]*recommendservice.RecommendPost) (string, []tool.Source, error) {
	if len(ids) == 0 {
		return "暂时没有可推荐的已发布帖子。", nil, nil
	}
	published, err := tool.PublishedPosts(ctx, content, ids)
	if err != nil {
		return "", nil, err
	}
	var text strings.Builder
	sources := make([]tool.Source, 0, 3)
	kept := 0
	for _, id := range ids {
		info := published[id]
		if info == nil {
			continue
		}
		reason := ""
		if meta != nil && meta[id] != nil {
			reason = strings.TrimSpace(meta[id].Reason)
		}
		snippet := truncateRunes(info.Content, maxEvidenceSnippetRunes)
		fmt.Fprintf(&text, "- [post:%d] %s\n  摘要: %s\n", info.Id, info.Title, snippet)
		if reason != "" {
			fmt.Fprintf(&text, "  推荐理由: %s\n", reason)
		}
		sources = append(sources, tool.Source{
			Type: "post", ID: strconv.FormatInt(info.Id, 10), Title: info.Title,
			Snippet: snippet, Revision: info.Revision,
		})
		kept++
		if kept >= 3 {
			break
		}
	}
	if kept == 0 {
		return "暂时没有可推荐的已发布帖子。", nil, nil
	}
	text.WriteString("候选均来自推荐/检索系统并已回源验证，不得编造帖子 ID。")
	return strings.TrimRight(text.String(), "\n"), sources, nil
}

func excludedPostIDs(ctx context.Context, store memory.Store, userID int64) map[int64]struct{} {
	out := map[int64]struct{}{}
	if store == nil || userID <= 0 {
		return out
	}
	items, err := store.List(ctx, userID, memory.LayerProfile, time.Now())
	if err != nil {
		logx.WithContext(ctx).Infow("recommend hard filter skipped", logx.Field("err", err.Error()))
		return out
	}
	for _, item := range items {
		if item.Dimension != "post" || item.Score >= 0 || item.Suppressed {
			continue
		}
		id, convErr := strconv.ParseInt(item.Value, 10, 64)
		if convErr != nil || id <= 0 {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}
