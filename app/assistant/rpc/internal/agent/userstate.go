package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"esx/app/assistant/rpc/internal/tool"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	ToolGetMyFavorites = "get_my_favorites"
	ToolGetMyLikes     = "get_my_likes"
	ToolGetMyFollowing = "get_my_following"
	ToolGetMyPosts     = "get_my_posts"
)

type userStatePageArgs struct {
	UserID   int64 `json:"user_id"`
	Page     int32 `json:"page"`
	PageSize int32 `json:"page_size"`
}

func sessionUserID(session *Session) (int64, error) {
	if session == nil || session.UserID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return session.UserID, nil
}

func normalizeUserStatePage(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 20 {
		pageSize = 10
	}
	return page, pageSize
}

func getMyFavoritesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args userStatePageArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_my_favorites arguments are invalid")
		}
		page, pageSize := normalizeUserStatePage(args.Page, args.PageSize)
		resp, err := clients.Interaction.GetFavoriteList(ctx, &interactionservice.GetFavoriteListReq{
			UserId: userID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserStatePosts(ctx, clients.Content, resp.GetPostIds(), resp.GetTotal(), "收藏")
	}
}

func getMyLikesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args userStatePageArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_my_likes arguments are invalid")
		}
		page, pageSize := normalizeUserStatePage(args.Page, args.PageSize)
		resp, err := clients.Interaction.GetLikeList(ctx, &interactionservice.GetLikeListReq{
			UserId: userID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserStatePosts(ctx, clients.Content, resp.GetPostIds(), resp.GetTotal(), "点赞")
	}
}

func getMyFollowingExecutor(user userservice.UserService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if user == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args userStatePageArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_my_following arguments are invalid")
		}
		page, pageSize := normalizeUserStatePage(args.Page, args.PageSize)
		resp, err := user.GetFollowing(ctx, &userservice.GetFollowingReq{
			UserId: userID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		users := resp.GetUsers()
		if len(users) == 0 {
			return "目前没有关注的人。", nil, nil
		}
		var text strings.Builder
		fmt.Fprintf(&text, "你关注了 %d 人（本页 %d）：\n", resp.GetTotal(), len(users))
		for _, info := range users {
			if info == nil || info.Id <= 0 {
				continue
			}
			name := strings.TrimSpace(info.Nickname)
			if name == "" {
				name = strings.TrimSpace(info.Username)
			}
			fmt.Fprintf(&text, "- user_id=%d %s\n", info.Id, name)
		}
		return strings.TrimRight(text.String(), "\n"), nil, nil
	}
}

func getMyPostsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args userStatePageArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_my_posts arguments are invalid")
		}
		_, pageSize := normalizeUserStatePage(args.Page, args.PageSize)
		resp, err := content.GetUserPosts(ctx, &contentservice.GetUserPostsReq{
			UserId: userID, PageSize: pageSize, SortBy: 1,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(resp.GetPosts()))
		for _, post := range resp.GetPosts() {
			if post == nil || post.Id <= 0 {
				continue
			}
			ids = append(ids, post.Id)
		}
		return formatUserStatePosts(ctx, content, ids, int64(len(ids)), "发布")
	}
}

func formatUserStatePosts(ctx context.Context, content contentservice.ContentService, ids []int64, total int64, kind string) (string, []tool.Source, error) {
	if len(ids) == 0 {
		return fmt.Sprintf("目前没有可展示的已发布%s帖子。", kind), nil, nil
	}
	published, err := tool.PublishedPosts(ctx, content, ids)
	if err != nil {
		logx.WithContext(ctx).Errorw("agent userstate backfill failed", logx.Field("err", err.Error()))
		return "", nil, err
	}
	var text strings.Builder
	sources := make([]tool.Source, 0, len(ids))
	fmt.Fprintf(&text, "你的已发布%s帖子（共 %d 条候选，仅展示当前可见已发布）：\n", kind, total)
	for _, id := range ids {
		info := published[id]
		if info == nil || !visibilityx.IsPublished(info.Status) {
			continue
		}
		snippet := truncateRunes(info.Content, 120)
		fmt.Fprintf(&text, "- [post:%d] %s\n  %s\n", info.Id, info.Title, snippet)
		sources = append(sources, tool.Source{
			Type: "post", ID: strconv.FormatInt(info.Id, 10), Title: info.Title,
			Snippet: snippet, Revision: info.Revision,
		})
	}
	if len(sources) == 0 {
		return fmt.Sprintf("目前没有可展示的已发布%s帖子。", kind), nil, nil
	}
	return strings.TrimRight(text.String(), "\n"), sources, nil
}
