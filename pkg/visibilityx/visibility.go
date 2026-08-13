package visibilityx

import (
	"context"

	"errx"
	"esx/pkg/validator"
)

// PublishedStatus is the authoritative content status for ordinary public reads.
const PublishedStatus int32 = 1

// Post is the minimum content view needed to decide public visibility.
type Post interface {
	GetId() int64
	GetStatus() int32
}

// Fetcher loads one ID batch from the content authority. A nil fetcher, a
// fetch error, or a nil batch is a fail-closed visibility failure.
type Fetcher[T Post] func(ctx context.Context, ids []int64) ([]T, error)

// IsPublished reports whether status is ordinary published content.
func IsPublished(status int32) bool {
	return status == PublishedStatus
}

// PublishedByIDs returns currently published posts keyed by ID.
// It deduplicates IDs, batches with validator.MaxBatchQueryIds, and drops
// unpublished or unknown IDs. Visibility cannot be verified if fetch fails.
func PublishedByIDs[T Post](ctx context.Context, fetch Fetcher[T], ids []int64) (map[int64]T, error) {
	if fetch == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	unique := uniquePositiveIDs(ids)
	published := make(map[int64]T, len(unique))
	if len(unique) == 0 {
		return published, nil
	}
	requested := make(map[int64]struct{}, len(unique))
	for _, id := range unique {
		requested[id] = struct{}{}
	}
	for start := 0; start < len(unique); start += validator.MaxBatchQueryIds {
		end := min(start+validator.MaxBatchQueryIds, len(unique))
		batch, err := fetch(ctx, unique[start:end])
		if err != nil {
			return nil, err
		}
		if batch == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		for _, post := range batch {
			id := post.GetId()
			if id <= 0 || !IsPublished(post.GetStatus()) {
				continue
			}
			if _, ok := requested[id]; !ok {
				continue
			}
			published[id] = post
		}
	}
	return published, nil
}

// AdjustPageTotal subtracts items removed from the current page only.
// It does not recount other pages.
func AdjustPageTotal(total int64, fetched, visible int) int64 {
	removed := int64(fetched - visible)
	if removed <= 0 {
		return total
	}
	if total < removed {
		return int64(visible)
	}
	return total - removed
}

func uniquePositiveIDs(ids []int64) []int64 {
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
