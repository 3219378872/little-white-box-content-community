package logic

import (
	"esx/app/content/rpc/internal/model"
	"esx/pkg/visibilityx"
)

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
