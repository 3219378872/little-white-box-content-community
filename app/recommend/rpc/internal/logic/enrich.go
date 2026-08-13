package logic

import (
	"context"
	"esx/app/recommend/rpc/internal/model"
	"fmt"
)

func enrichAndFilterPosts(
	ctx context.Context,
	repository model.FeatureRepository,
	identity string,
	candidates []model.PostCandidate,
	excludedPostID int64,
	skipViewer bool,
) ([]model.PostCandidate, bool, error) {
	viewer := emptyViewerFeatures()
	degraded := false
	if repository == nil {
		degraded = true
	} else if !skipViewer {
		loadedViewer, err := repository.LoadViewerFeatures(ctx, identity)
		if err != nil {
			degraded = true
		} else {
			viewer = loadedViewer
		}
	}

	postFeatures := make(map[int64]model.PostFeatures, len(candidates))
	if repository == nil {
		degraded = true
	} else {
		postIDs := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			postIDs = append(postIDs, candidate.PostID)
		}
		loaded, err := repository.LoadPostFeatures(ctx, postIDs)
		if err != nil {
			degraded = true
		} else {
			postFeatures = loaded
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, degraded, err
	}

	result := make([]model.PostCandidate, 0, len(candidates))
	var seenExcluded []model.PostCandidate
	for _, candidate := range candidates {
		if candidate.PostID == excludedPostID || containsID(viewer.PositivePostIDs, candidate.PostID) ||
			containsID(viewer.NegativePostIDs, candidate.PostID) {
			continue
		}
		features := candidate.Features
		if loaded, ok := postFeatures[candidate.PostID]; ok && loaded.Known {
			features = loaded
		}
		if !features.Known || !features.Available || !isPublic(features.Visibility) {
			continue
		}
		if _, blocked := viewer.BlockedAuthors[features.AuthorID]; blocked && features.AuthorID > 0 {
			continue
		}
		candidate.Features = features
		candidate.AuthorID = features.AuthorID
		candidate.Category = features.Category
		if containsID(viewer.SeenPostIDs, candidate.PostID) {
			// DISC-035：已曝光内容优先排除；保留用于候选不足时重新进入。
			seenExcluded = append(seenExcluded, candidate)
			continue
		}
		result = append(result, candidate)
	}
	// DISC-035：排除后候选不足时允许已曝光内容重新进入，并标记原因。
	if len(result) == 0 && len(seenExcluded) > 0 {
		for index := range seenExcluded {
			seenExcluded[index].Reason = appendSource(seenExcluded[index].Reason, "re-entered after exposure window")
		}
		result = seenExcluded
	}
	if len(result) == 0 && len(candidates) > 0 && degraded {
		return nil, true, fmt.Errorf("feature repository unavailable and no verified post candidates remain")
	}
	return result, degraded, nil
}

func enrichAndFilterUsers(
	ctx context.Context,
	repository model.FeatureRepository,
	requestUserID int64,
	candidates []model.UserCandidate,
) ([]model.UserCandidate, bool, error) {
	featuresByUser := make(map[int64]model.UserFeatures, len(candidates))
	degraded := false
	if repository == nil {
		degraded = true
	} else {
		userIDs := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			userIDs = append(userIDs, candidate.UserID)
		}
		loaded, err := repository.LoadUserFeatures(ctx, userIDs)
		if err != nil {
			degraded = true
		} else {
			featuresByUser = loaded
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, degraded, err
	}
	result := make([]model.UserCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.UserID == requestUserID {
			continue
		}
		features := candidate.Features
		if loaded, ok := featuresByUser[candidate.UserID]; ok && loaded.Known {
			features = loaded
		}
		if !features.Known || !features.Available || !isPublic(features.Visibility) {
			continue
		}
		candidate.Features = features
		candidate.Category = features.Category
		result = append(result, candidate)
	}
	if len(result) == 0 && len(candidates) > 0 && degraded {
		return nil, true, fmt.Errorf("feature repository unavailable and no verified user candidates remain")
	}
	return result, degraded, nil
}
