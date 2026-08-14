package logic

import (
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"sort"
	"strconv"
	"strings"
)

// PostToPostInfo 将 Post model 转换为 pb.PostInfo
func PostToPostInfo(post *model2.Post, tags []string) *pb.PostInfo {
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
		Revision:      post.Revision,
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		CreatedAt:     post.CreatedAt.UnixMilli(),
		UpdatedAt:     post.UpdatedAt.UnixMilli(),
	}
}

// CommentToCommentInfo 将 Comment model 转换为 pb.CommentInfo
func CommentToCommentInfo(comment *model2.Comment) *pb.CommentInfo {
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
func TagToTagInfo(tag *model2.Tag) *pb.TagInfo {
	return &pb.TagInfo{
		Id:        tag.Id,
		Name:      tag.Name,
		PostCount: tag.PostCount,
	}
}

// sortedMediaIDs 返回排序去重后的媒体 ID 字符串列表，用于幂等命令哈希：
// 同一批媒体顺序无关（CORE-050 同命令语义）。
func sortedMediaIDs(ids []int64) []string {
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
	out := make([]string, 0, len(unique))
	for _, id := range unique {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out
}
