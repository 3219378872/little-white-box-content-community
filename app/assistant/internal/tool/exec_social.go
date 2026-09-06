package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"fmt"
	"strings"
)

func getMyFavoritesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := clients.Interaction.GetFavoriteList(ctx, &interactionservice.GetFavoriteListReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserPosts(ctx, clients.Content, resp.GetPostIds(), "收藏")
	}
}

func getMyLikesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := clients.Interaction.GetLikeList(ctx, &interactionservice.GetLikeListReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserPosts(ctx, clients.Content, resp.GetPostIds(), "点赞")
	}
}

func getMyFollowingExecutor(user userservice.UserService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if user == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := user.GetFollowing(ctx, &userservice.GetFollowingReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(resp.GetUsers()) == 0 {
			return "目前没有关注的人。", nil, nil
		}
		var b strings.Builder
		for _, info := range resp.GetUsers() {
			if info != nil {
				fmt.Fprintf(&b, "- user_id=%d %s\n", info.Id, info.Nickname)
			}
		}
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}
