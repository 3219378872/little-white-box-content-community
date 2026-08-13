package logic

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/pkg/visibilityx"
)

const (
	maxFeedPageSize   = 100
	feedTypeFollow    = 1
	feedTypeRecommend = 2
)

func fetchContentPosts(contentService svc.ContentService) visibilityx.Fetcher[*contentservice.PostInfo] {
	return func(ctx context.Context, ids []int64) ([]*contentservice.PostInfo, error) {
		if contentService == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		response, err := contentService.GetPostsByIds(ctx, &contentservice.GetPostsByIdsReq{PostIds: ids})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		return response.Posts, nil
	}
}

func enrichFeedItems(ctx context.Context, contentService svc.ContentService, baseItems []*pb.FeedItem) ([]*pb.FeedItem, error) {
	postIDs := make([]int64, 0, len(baseItems))
	for _, item := range baseItems {
		if item == nil {
			continue
		}
		postIDs = append(postIDs, item.PostId)
	}
	postsByID, err := visibilityx.PublishedByIDs(ctx, fetchContentPosts(contentService), postIDs)
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

func fallbackFeedItem(post *contentservice.PostInfo, source string) *pb.FeedItem {
	if post == nil || post.Id <= 0 || !visibilityx.IsPublished(post.Status) {
		return nil
	}
	return renderFeedItem(&pb.FeedItem{
		PostId:       post.Id,
		FeedType:     feedTypeRecommend,
		Reason:       source + " fallback",
		RecallSource: source,
		ModelVersion: "rule-fallback-v2",
	}, post)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
