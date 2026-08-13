package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"esx/pkg/validator"
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

// publishedSearchPosts 在返回搜索结果前用 Content 权威状态过滤不可见帖子。
// 可见性无法验证时整体失败（DISC-001 / DISC-041 / REL-042）。
func publishedSearchPosts(ctx context.Context, content svc.ContentService, posts []store.Post) ([]store.Post, error) {
	if len(posts) == 0 {
		return posts, nil
	}
	if content == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	ids := make([]int64, 0, len(posts))
	seen := make(map[int64]struct{}, len(posts))
	for _, post := range posts {
		if post.ID <= 0 {
			continue
		}
		if _, exists := seen[post.ID]; exists {
			continue
		}
		seen[post.ID] = struct{}{}
		ids = append(ids, post.ID)
	}
	published := make(map[int64]struct{}, len(ids))
	for start := 0; start < len(ids); start += validator.MaxBatchQueryIds {
		end := min(start+validator.MaxBatchQueryIds, len(ids))
		response, err := content.GetPostsByIds(ctx, &contentservice.GetPostsByIdsReq{PostIds: ids[start:end]})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		for _, post := range response.Posts {
			if post != nil && post.Id > 0 && post.Status == 1 {
				published[post.Id] = struct{}{}
			}
		}
	}
	filtered := make([]store.Post, 0, len(posts))
	emitted := make(map[int64]struct{}, len(published))
	for _, post := range posts {
		if _, ok := published[post.ID]; !ok {
			continue
		}
		if _, exists := emitted[post.ID]; exists {
			continue
		}
		filtered = append(filtered, post)
		emitted[post.ID] = struct{}{}
	}
	return filtered, nil
}
