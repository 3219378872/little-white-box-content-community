package router

import (
	"context"
	"fmt"

	"esx/app/review/internal/store"
)

// SeedSource 是种子同步需要的权威存储。
type SeedSource interface {
	SeedsToSync(ctx context.Context, limit int) ([]store.Seed, error)
	MarkSeedIndex(ctx context.Context, id int64, status, indexState string) error
}

// SeedWriter 是可写的种子集合。
type SeedWriter interface {
	Insert(ctx context.Context, seedID int64, market, issue string, vector []float32) error
	Delete(ctx context.Context, seedIDs []int64) error
}

// SyncSeeds 把确认的种子写入集合、把停用的种子移出集合。权威状态先变化，召回时再复核，
// 所以同步延迟不会让停用种子继续命中（RVW-030）。
func SyncSeeds(ctx context.Context, source SeedSource, embedder Embedder, writer SeedWriter, limit int) (int, error) {
	seeds, err := source.SeedsToSync(ctx, limit)
	if err != nil {
		return 0, err
	}
	synced := 0
	for _, seed := range seeds {
		switch seed.Status {
		case store.SeedActive:
			vector, _, err := embedder.Embed(ctx, seed.Text)
			if err != nil {
				return synced, fmt.Errorf("router: embed seed %d: %w", seed.ID, err)
			}
			if err := writer.Insert(ctx, seed.ID, seed.Market, seed.IssueCode, vector); err != nil {
				return synced, err
			}
			if err := source.MarkSeedIndex(ctx, seed.ID, store.SeedActive, store.SeedIndexed); err != nil {
				return synced, err
			}
		case store.SeedRetired:
			if err := writer.Delete(ctx, []int64{seed.ID}); err != nil {
				return synced, err
			}
			if err := source.MarkSeedIndex(ctx, seed.ID, store.SeedRetired, store.SeedRemoved); err != nil {
				return synced, err
			}
		}
		synced++
	}
	return synced, nil
}
