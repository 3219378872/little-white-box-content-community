package logic

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/feed/rpc/internal/svc"
	pb "esx/kitex_gen/feed"
)

// 单页上限与 FeedItem.FeedType 取值：1=关注流，2=推荐流。
const (
	maxFeedPageSize   = 100
	feedTypeFollow    = 1
	feedTypeRecommend = 2
)

// enrichFeedItems 用内容服务补全帖子详情，只保留已发布的帖子并按帖子去重，保持输入顺序。
func enrichFeedItems(ctx context.Context, contentService svc.ContentService, baseItems []*pb.FeedItem) ([]*pb.FeedItem, error) {
	postIDs := make([]int64, 0, len(baseItems))
	for _, item := range baseItems {
		if item == nil {
			continue
		}
		postIDs = append(postIDs, item.PostId)
	}
	postsByID, err := visibility.PublishedByIDs(ctx, contentService, postIDs)
	if err != nil {
		return nil, err
	}

	items := make([]*pb.FeedItem, 0, len(baseItems))
	emitted := make(map[int64]struct{}, len(postsByID))
	for _, item := range baseItems {
		if item == nil {
			continue
		}
		if _, exists := emitted[item.PostId]; exists {
			continue
		}
		post := postsByID[item.PostId]
		if post == nil {
			continue
		}
		items = append(items, renderFeedItem(item, post))
		emitted[item.PostId] = struct{}{}
	}
	return items, nil
}

// renderFeedItem 合并推荐元数据与帖子详情；作者与创建时间以内容服务的权威值为准。
func renderFeedItem(base *pb.FeedItem, post *contentservice.PostInfo) *pb.FeedItem {
	return &pb.FeedItem{
		PostId:        base.PostId,
		AuthorId:      post.AuthorId,
		CreatedAt:     post.CreatedAt,
		FeedType:      base.FeedType,
		Score:         base.Score,
		Reason:        base.Reason,
		RecallSource:  base.RecallSource,
		ModelVersion:  base.ModelVersion,
		ExperimentId:  base.ExperimentId,
		Position:      base.Position,
		Title:         post.Title,
		Content:       post.Content,
		Images:        cloneStrings(post.Images),
		Tags:          cloneStrings(post.Tags),
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
	}
}

// cloneStrings 复制切片，并把空值规范为空数组，避免响应序列化出 null。
func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
