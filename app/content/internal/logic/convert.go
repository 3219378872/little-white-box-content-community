package logic

import (
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"strings"
)

// PostToPostInfo 将 Post model 转换为 pb.PostInfo
func PostToPostInfo(post *model.Post, tags []string) *pb.PostInfo {
	var images []string
	if post.Images.Valid && post.Images.String != "" {
		images = strings.Split(post.Images.String, ",")
	} else {
		images = []string{}
	}

	if tags == nil {
		tags = []string{}
	}

	return &pb.PostInfo{
		Id:            post.Id,
		AuthorId:      post.AuthorId,
		Title:         post.Title,
		Content:       post.Content,
		Images:        images,
		Tags:          tags,
		Status:        int32(post.Status),
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		CreatedAt:     post.CreatedAt.UnixMilli(),
		UpdatedAt:     post.UpdatedAt.UnixMilli(),
	}
}

// CommentToCommentInfo 将 Comment model 转换为 pb.CommentInfo
func CommentToCommentInfo(comment *model.Comment) *pb.CommentInfo {
	var parentId int64
	if comment.ParentId.Valid {
		parentId = comment.ParentId.Int64
	}

	var replyUserId int64
	if comment.ReplyUserId.Valid {
		replyUserId = comment.ReplyUserId.Int64
	}

	return &pb.CommentInfo{
		Id:          comment.Id,
		PostId:      comment.PostId,
		UserId:      comment.UserId,
		ParentId:    parentId,
		ReplyUserId: replyUserId,
		Content:     comment.Content,
		Status:      int32(comment.Status),
		LikeCount:   comment.LikeCount,
		CreatedAt:   comment.CreatedAt.UnixMilli(),
	}
}

// TagToTagInfo 将 Tag model 转换为 pb.TagInfo
func TagToTagInfo(tag *model.Tag) *pb.TagInfo {
	return &pb.TagInfo{
		Id:        tag.Id,
		Name:      tag.Name,
		PostCount: tag.PostCount,
	}
}
