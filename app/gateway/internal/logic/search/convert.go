package search

import (
	"strings"

	"esx/app/gateway/internal/types"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"
)

const maxPageSize = 100

// searchKeyword 去除首尾空白，空关键词返回 SearchEmpty 以便前端提示。
func searchKeyword(keyword string) (string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", errx.NewWithCode(errx.SearchEmpty)
	}
	return keyword, nil
}

// validPage 校验页码从 1 开始且页大小不超过上限。
func validPage(page, pageSize int32) bool {
	return page > 0 && pageSize > 0 && pageSize <= maxPageSize
}

// searchPosts 把帖子搜索结果映射为 REST 项，跳过 nil 结果。
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
			AuthorId:         post.AuthorId,
			AuthorName:       post.AuthorName,
			AuthorAvatar:     post.AuthorAvatar,
			LikeCount:        post.LikeCount,
			CommentCount:     post.CommentCount,
			CreatedAt:        post.CreatedAt,
		})
	}
	return items
}

// searchUsers 把用户搜索结果映射为 REST 项，跳过 nil 结果。
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

// searchTags 把标签搜索结果映射为 REST 项，跳过 nil 结果。
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
