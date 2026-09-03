// Package authorx loads display names for post/feed authors from User RPC.
package authorx

import (
	"context"
	"strings"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type Author struct {
	Name   string
	Avatar string
}

func DisplayName(nickname, username string) string {
	name := strings.TrimSpace(nickname)
	if name == "" {
		name = strings.TrimSpace(username)
	}
	return name
}

func UniquePositive(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func PostAuthorIDs(posts []*contentservice.PostInfo) []int64 {
	ids := make([]int64, 0, len(posts))
	for _, post := range posts {
		if post == nil {
			continue
		}
		ids = append(ids, post.AuthorId)
	}
	return UniquePositive(ids)
}

// Load batch-fetches authors. Missing UserService or a failed RPC is an error
// (feed fail-closed). Unknown IDs are omitted from the map.
func Load(ctx context.Context, svcCtx *svc.ServiceContext, ids []int64) (map[int64]Author, error) {
	unique := UniquePositive(ids)
	authors := make(map[int64]Author, len(unique))
	if len(unique) == 0 {
		return authors, nil
	}
	if svcCtx == nil || svcCtx.UserService == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	response, err := svcCtx.UserService.BatchGetUsers(ctx, &userservice.BatchGetUsersReq{UserIds: unique})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	if response == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	for _, user := range response.Users {
		if user == nil || user.Id <= 0 {
			continue
		}
		authors[user.Id] = Author{
			Name:   DisplayName(user.Nickname, user.Username),
			Avatar: strings.TrimSpace(user.AvatarUrl),
		}
	}
	return authors, nil
}

// LoadSoft is fail-open: missing service or lookup failure yields empty names.
func LoadSoft(ctx context.Context, svcCtx *svc.ServiceContext, ids []int64) map[int64]Author {
	unique := UniquePositive(ids)
	if len(unique) == 0 {
		return map[int64]Author{}
	}
	if svcCtx == nil || svcCtx.UserService == nil {
		return map[int64]Author{}
	}
	authors, err := Load(ctx, svcCtx, unique)
	if err != nil {
		logx.WithContext(ctx).Errorw("UserService.BatchGetUsers failed",
			logx.Field("authorIds", unique),
			logx.Field("err", err.Error()),
		)
		return map[int64]Author{}
	}
	return authors
}
