package user

import (
	"context"
	"strings"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	"esx/app/user/rpc/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type postAuthor struct {
	name   string
	avatar string
}

func uniquePostAuthorIDs(posts []*contentservice.PostInfo) []int64 {
	authorIDs := make([]int64, 0, len(posts))
	seen := make(map[int64]struct{}, len(posts))
	for _, post := range posts {
		if post == nil || post.AuthorId <= 0 {
			continue
		}
		if _, ok := seen[post.AuthorId]; ok {
			continue
		}
		seen[post.AuthorId] = struct{}{}
		authorIDs = append(authorIDs, post.AuthorId)
	}
	return authorIDs
}

func loadPostAuthors(ctx context.Context, svcCtx *svc.ServiceContext, authorIDs []int64) map[int64]postAuthor {
	if len(authorIDs) == 0 || svcCtx == nil || svcCtx.UserService == nil {
		return nil
	}
	response, err := svcCtx.UserService.BatchGetUsers(ctx, &userservice.BatchGetUsersReq{UserIds: authorIDs})
	if err != nil {
		logx.WithContext(ctx).Errorw("UserService.BatchGetUsers failed",
			logx.Field("authorIds", authorIDs),
			logx.Field("err", err.Error()),
		)
		return nil
	}
	if response == nil {
		logx.WithContext(ctx).Error("UserService.BatchGetUsers returned a nil response")
		return nil
	}

	authors := make(map[int64]postAuthor, len(response.Users))
	for _, user := range response.Users {
		if user == nil || user.Id <= 0 {
			continue
		}
		name := strings.TrimSpace(user.Nickname)
		if name == "" {
			name = strings.TrimSpace(user.Username)
		}
		authors[user.Id] = postAuthor{
			name:   name,
			avatar: strings.TrimSpace(user.AvatarUrl),
		}
	}
	return authors
}
