package logic

import (
	"context"
	"errors"
	"esx/app/recommend/rpc/internal/model"
	"fmt"
	"sort"
	"sync"
)

func recallPosts(ctx context.Context, sources []model.PostRecallSource, req model.RecallRequest) ([][]model.PostCandidate, bool, error) {
	if len(sources) == 0 {
		return nil, false, fmt.Errorf("no post recall sources configured")
	}
	results := make(chan postRecallResult, len(sources))
	var group sync.WaitGroup
	for index, source := range sources {
		if source == nil {
			continue
		}
		group.Add(1)
		go func(index int, source model.PostRecallSource) {
			defer group.Done()
			candidates, err := source.Recall(ctx, req)
			for candidateIndex := range candidates {
				if candidates[candidateIndex].RecallSource == "" {
					candidates[candidateIndex].RecallSource = source.Name()
				}
			}
			results <- postRecallResult{index: index, candidates: candidates, err: err}
		}(index, source)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	batches := make([][]model.PostCandidate, len(sources))
	var failures []error
	successes := 0
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, model.ErrNotApplicable) {
				failures = append(failures, result.err)
			}
			continue
		}
		successes++
		batches[result.index] = result.candidates
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if successes == 0 {
		if len(failures) == 0 {
			return nil, false, fmt.Errorf("no applicable post recall sources")
		}
		return nil, false, errors.Join(failures...)
	}
	return batches, len(failures) > 0, nil
}

func recallUsers(ctx context.Context, sources []model.UserRecallSource, req model.RecallRequest) ([][]model.UserCandidate, bool, error) {
	if len(sources) == 0 {
		return nil, false, fmt.Errorf("no user recall sources configured")
	}
	results := make(chan userRecallResult, len(sources))
	var group sync.WaitGroup
	for index, source := range sources {
		if source == nil {
			continue
		}
		group.Add(1)
		go func(index int, source model.UserRecallSource) {
			defer group.Done()
			candidates, err := source.Recall(ctx, req)
			for candidateIndex := range candidates {
				if candidates[candidateIndex].RecallSource == "" {
					candidates[candidateIndex].RecallSource = source.Name()
				}
			}
			results <- userRecallResult{index: index, candidates: candidates, err: err}
		}(index, source)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	batches := make([][]model.UserCandidate, len(sources))
	var failures []error
	successes := 0
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, model.ErrNotApplicable) {
				failures = append(failures, result.err)
			}
			continue
		}
		successes++
		batches[result.index] = result.candidates
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if successes == 0 {
		if len(failures) == 0 {
			return nil, false, fmt.Errorf("no applicable user recall sources")
		}
		return nil, false, errors.Join(failures...)
	}
	return batches, len(failures) > 0, nil
}

func mergePostCandidates(batches [][]model.PostCandidate, limit int) []model.PostCandidate {
	merged := make(map[int64]model.PostCandidate)
	for _, batch := range batches {
		for rank, candidate := range batch {
			if candidate.PostID <= 0 {
				continue
			}
			contribution := 1 / (rrfConstant + float64(rank+1))
			current, exists := merged[candidate.PostID]
			if !exists {
				candidate.RecallScore = contribution
				merged[candidate.PostID] = candidate
				continue
			}
			current.RecallScore += contribution
			current.RecallSource = appendSource(current.RecallSource, candidate.RecallSource)
			if !current.Features.Known && candidate.Features.Known {
				current.Features = candidate.Features
				current.AuthorID = candidate.AuthorID
				current.Category = candidate.Category
			}
			merged[candidate.PostID] = current
		}
	}
	result := make([]model.PostCandidate, 0, len(merged))
	for _, candidate := range merged {
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RecallScore == result[j].RecallScore {
			return result[i].PostID < result[j].PostID
		}
		return result[i].RecallScore > result[j].RecallScore
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func mergeUserCandidates(batches [][]model.UserCandidate, limit int) []model.UserCandidate {
	merged := make(map[int64]model.UserCandidate)
	for _, batch := range batches {
		for rank, candidate := range batch {
			if candidate.UserID <= 0 {
				continue
			}
			contribution := 1 / (rrfConstant + float64(rank+1))
			current, exists := merged[candidate.UserID]
			if !exists {
				candidate.RecallScore = contribution
				merged[candidate.UserID] = candidate
				continue
			}
			current.RecallScore += contribution
			current.RecallSource = appendSource(current.RecallSource, candidate.RecallSource)
			if !current.Features.Known && candidate.Features.Known {
				current.Features = candidate.Features
				current.Category = candidate.Category
			}
			merged[candidate.UserID] = current
		}
	}
	result := make([]model.UserCandidate, 0, len(merged))
	for _, candidate := range merged {
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RecallScore == result[j].RecallScore {
			return result[i].UserID < result[j].UserID
		}
		return result[i].RecallScore > result[j].RecallScore
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}
