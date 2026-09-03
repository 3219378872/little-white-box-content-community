package logic

import (
	"encoding/json"

	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"slices"
	"strconv"
	"strings"
)

// PostToPostInfo 将 Post model 转换为 pb.PostInfo
func PostToPostInfo(post *model2.Post, tags []string) *pb.PostInfo {
	images := decodeStringSlice(post.Images.String, post.Images.Valid)
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
		MediaIds:      decodeInt64sJSON(post.MediaIds),
	}
}

func decodeStringSlice(raw string, valid bool) []string {
	if !valid || raw == "" {
		return []string{}
	}
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") {
		var images []string
		if err := json.Unmarshal([]byte(trimmed), &images); err == nil {
			return images
		}
	}
	return strings.Split(raw, ",")
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
		ReplyCount:  comment.ReplyCount,
	}
}

// previewReplyLimit 评论列表内嵌的回复预览条数上限。
const previewReplyLimit = 3

// groupReplyPreviews 将按时间正序返回的回复行按父评论归组，
// 每组截取前 previewReplyLimit 条作为内嵌预览。
// 输入全局正序，因此每组子序列天然保持正序。
func groupReplyPreviews(replies []*model2.Comment) map[int64][]*pb.CommentInfo {
	out := make(map[int64][]*pb.CommentInfo, len(replies))
	for _, reply := range replies {
		if !reply.ParentId.Valid {
			continue
		}
		parentId := reply.ParentId.Int64
		if len(out[parentId]) >= previewReplyLimit {
			continue
		}
		out[parentId] = append(out[parentId], CommentToCommentInfo(reply))
	}
	return out
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
	slices.Sort(unique)
	out := make([]string, 0, len(unique))
	for _, id := range unique {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out
}
