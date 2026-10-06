package logic

import (
	"esx/app/content/rpc/internal/model"
	"esx/pkg/visibilityx"
)

// indexPublishedPosts 按 ID 索引已发布的帖子，过滤不可见的行。
func indexPublishedPosts(posts []*model.Post) map[int64]*model.Post {
	out := make(map[int64]*model.Post, len(posts))
	for _, post := range posts {
		if post == nil || post.Id <= 0 || !visibilityx.IsPublished(int32(post.Status)) {
			continue
		}
		out[post.Id] = post
	}
	return out
}

// keepPublishedPosts 保留已发布的帖子并保持原顺序。
func keepPublishedPosts(posts []*model.Post) []*model.Post {
	out := make([]*model.Post, 0, len(posts))
	for _, post := range posts {
		if post == nil || post.Id <= 0 || !visibilityx.IsPublished(int32(post.Status)) {
			continue
		}
		out = append(out, post)
	}
	return out
}

const commentActiveStatus int64 = 1

// keepActiveComments 保留有效评论并保持原顺序。
func keepActiveComments(comments []*model.Comment) []*model.Comment {
	out := make([]*model.Comment, 0, len(comments))
	for _, comment := range comments {
		if comment == nil || comment.Id <= 0 || comment.Status != commentActiveStatus {
			continue
		}
		out = append(out, comment)
	}
	return out
}
