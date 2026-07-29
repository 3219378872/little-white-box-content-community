package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"errx"
	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"user/userservice"
)

const maxPageSize = 100

func keyword(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errx.NewWithCode(errx.SearchEmpty)
	}
	return value, nil
}

func validPage(page, pageSize int32) bool {
	if page <= 0 || pageSize <= 0 || pageSize > maxPageSize {
		return false
	}
	offset := int64(page-1) * int64(pageSize)
	return offset < store.MaxResultWindow && offset+int64(pageSize) <= store.MaxResultWindow
}

func storeError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errx.Wrap(err, errx.SearchTimeout)
	}
	return errx.Wrap(err, errx.ServiceUnavailable)
}

func loadUserProfiles(
	ctx context.Context,
	userService svc.UserService,
	posts []store.Post,
) (map[int64]*userservice.UserInfo, error) {
	ids := make([]int64, 0, len(posts))
	seen := make(map[int64]struct{}, cap(ids))
	appendID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, post := range posts {
		appendID(post.AuthorID)
	}
	if len(ids) == 0 {
		return map[int64]*userservice.UserInfo{}, nil
	}
	if userService == nil {
		return nil, fmt.Errorf("search user service is unavailable")
	}
	response, err := userService.BatchGetUsers(ctx, &userservice.BatchGetUsersReq{UserIds: ids})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("user service returned a nil response")
	}
	profiles := make(map[int64]*userservice.UserInfo, len(response.Users))
	for _, user := range response.Users {
		if user != nil && user.Id > 0 {
			profiles[user.Id] = user
		}
	}
	return profiles, nil
}

func postResults(posts []store.Post, profiles map[int64]*userservice.UserInfo) []*pb.PostSearchResult {
	result := make([]*pb.PostSearchResult, 0, len(posts))
	for _, post := range posts {
		authorName := ""
		if profile := profiles[post.AuthorID]; profile != nil {
			authorName = strings.TrimSpace(profile.Nickname)
			if authorName == "" {
				authorName = profile.Username
			}
		}
		result = append(result, &pb.PostSearchResult{
			Id: post.ID, Title: post.Title, ContentHighlight: post.ContentHighlight,
			AuthorName: authorName, LikeCount: post.LikeCount,
			CommentCount: post.CommentCount, CreatedAt: post.CreatedAt,
		})
	}
	return result
}

func userResults(users []*userservice.UserInfo) []*pb.UserSearchResult {
	result := make([]*pb.UserSearchResult, 0, len(users))
	for _, user := range users {
		if user == nil || user.Id <= 0 {
			continue
		}
		result = append(result, &pb.UserSearchResult{
			Id: user.Id, Username: user.Username, Nickname: user.Nickname,
			AvatarUrl: user.AvatarUrl, Bio: user.Bio, FollowerCount: user.FollowerCount,
		})
	}
	return result
}

func tagResults(tags []store.Tag) []*pb.TagSearchResult {
	result := make([]*pb.TagSearchResult, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &pb.TagSearchResult{Name: tag.Name, PostCount: tag.PostCount})
	}
	return result
}
