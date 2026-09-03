// Package postmap maps Content PostInfo onto gateway post DTOs.
package postmap

import (
	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/types"
)

func Item(post *contentservice.PostInfo, liked, favorited map[int64]bool, author authorx.Author) types.PostItem {
	if post == nil {
		return types.PostItem{}
	}
	return types.PostItem{
		Id:            post.Id,
		AuthorId:      post.AuthorId,
		AuthorName:    author.Name,
		AuthorAvatar:  author.Avatar,
		Title:         post.Title,
		Content:       post.Content,
		Images:        post.Images,
		Tags:          post.Tags,
		Status:        post.Status,
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		IsLiked:       liked[post.Id],
		IsFavorited:   favorited[post.Id],
		Revision:      post.Revision,
		CreatedAt:     post.CreatedAt,
	}
}

func Detail(post *contentservice.PostInfo, liked, favorited map[int64]bool, author authorx.Author) types.GetPostResp {
	return types.GetPostResp(Item(post, liked, favorited, author))
}

func Items(posts []*contentservice.PostInfo, liked, favorited map[int64]bool, authors map[int64]authorx.Author) []types.PostItem {
	if authors == nil {
		authors = map[int64]authorx.Author{}
	}
	list := make([]types.PostItem, 0, len(posts))
	for _, post := range posts {
		if post == nil {
			continue
		}
		list = append(list, Item(post, liked, favorited, authors[post.AuthorId]))
	}
	return list
}
