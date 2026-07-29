package search

import (
	"errors"
	"strings"

	"errx"
	"esx/app/search/rpc/searchservice"
	"gateway/internal/types"
)

const maxPageSize = 100

func searchKeyword(keyword string) (string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", errx.NewWithCode(errx.SearchEmpty)
	}
	return keyword, nil
}

func validPage(page, pageSize int32) bool {
	return page > 0 && pageSize > 0 && pageSize <= maxPageSize
}

func searchRPCError(err error) error {
	var bizErr *errx.BizError
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return errx.Wrap(err, errx.SystemError)
}

func searchPosts(posts []*searchservice.PostSearchResult) []types.SearchPostItem {
	items := make([]types.SearchPostItem, 0, len(posts))
	for _, post := range posts {
		if post == nil {
			continue
		}
		items = append(items, types.SearchPostItem{
			Id:               post.Id,
			Title:            post.Title,
			ContentHighlight: post.ContentHighlight,
			AuthorName:       post.AuthorName,
			LikeCount:        post.LikeCount,
			CommentCount:     post.CommentCount,
			CreatedAt:        post.CreatedAt,
		})
	}
	return items
}

func searchUsers(users []*searchservice.UserSearchResult) []types.SearchUserItem {
	items := make([]types.SearchUserItem, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		items = append(items, types.SearchUserItem{
			Id:            user.Id,
			Username:      user.Username,
			Nickname:      user.Nickname,
			AvatarUrl:     user.AvatarUrl,
			Bio:           user.Bio,
			FollowerCount: user.FollowerCount,
		})
	}
	return items
}

func searchTags(tags []*searchservice.TagSearchResult) []types.SearchTagItem {
	items := make([]types.SearchTagItem, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		items = append(items, types.SearchTagItem{
			Name:      tag.Name,
			PostCount: tag.PostCount,
		})
	}
	return items
}
