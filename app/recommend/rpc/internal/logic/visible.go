package logic

import (
	"context"
	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/recommend/rpc/internal/model"
)

// publishedPostIDs 返回仍已发布的帖子 ID 集合，以内容服务为准。
func publishedPostIDs(ctx context.Context, content contentservice.ContentService, ids []int64) (map[int64]struct{}, error) {
	live, err := visibility.PublishedByIDs(ctx, content, ids)
	if err != nil {
		return nil, err
	}
	published := make(map[int64]struct{}, len(live))
	for id := range live {
		published[id] = struct{}{}
	}
	return published, nil
}

// filterPublishedPostCandidates 只保留仍已发布的候选，保持原顺序。
func filterPublishedPostCandidates(ctx context.Context, content contentservice.ContentService, candidates []model.PostCandidate) ([]model.PostCandidate, error) {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.PostID)
	}
	published, err := publishedPostIDs(ctx, content, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.PostCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := published[candidate.PostID]; ok {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

// filterPublishedRankedPosts 只保留仍已发布的快照结果，保持原顺序；翻页时过滤快照生成后下线的帖子。
func filterPublishedRankedPosts(ctx context.Context, content contentservice.ContentService, posts []model.RankedPost) ([]model.RankedPost, error) {
	ids := make([]int64, 0, len(posts))
	for _, post := range posts {
		ids = append(ids, post.PostID)
	}
	published, err := publishedPostIDs(ctx, content, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.RankedPost, 0, len(posts))
	for _, post := range posts {
		if _, ok := published[post.PostID]; ok {
			filtered = append(filtered, post)
		}
	}
	return filtered, nil
}
