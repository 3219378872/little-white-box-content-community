package svc

import (
	"context"
	"time"

	"esx/app/review/internal/intake"
	"esx/app/review/internal/store"
	"esx/pkg/event"
)

// Store 是 logic 用到的审核存储能力；生产使用 *store.Store，单测替换为内存实现。
type Store interface {
	Reviewer(ctx context.Context, userID int64) (*store.Reviewer, error)
	ClaimHuman(ctx context.Context, reviewer *store.Reviewer, purpose string, now time.Time) (*store.Task, error)
	Renew(ctx context.Context, reviewerID, taskID, generation int64, now time.Time) (*store.Task, error)
	Release(ctx context.Context, reviewerID, taskID, generation int64, now time.Time) error
	GetTask(ctx context.Context, id int64) (*store.Task, error)
	TaskSnapshotContent(ctx context.Context, taskID int64) (*store.Task, string, error)
	Snapshot(ctx context.Context, hash string) (string, error)
	Stages(ctx context.Context, taskID int64) ([]store.Stage, error)
	DecisionByTask(ctx context.Context, taskID int64) (*store.Decision, error)
	SubmitHuman(ctx context.Context, reviewerID, taskID, generation int64, idempotencyKey string,
		in store.DecisionInput, now time.Time) (*store.Decision, error)
	QueueSummary(ctx context.Context, reviewer *store.Reviewer, now time.Time) ([]store.QueueBucket, error)
	ListSeeds(ctx context.Context, status string, limit int) ([]store.Seed, error)
	TransitionSeed(ctx context.Context, seedID, actor int64, to string, now time.Time) (*store.Seed, error)
}

// Ingester 是送审入口；生产使用 *intake.Ingester，与 review-worker 消费共用同一实现。
type Ingester interface {
	Ingest(ctx context.Context, sub event.ReviewSubmittedEvent) (store.IngestResult, error)
}

var (
	_ Store    = (*store.Store)(nil)
	_ Ingester = (*intake.Ingester)(nil)
)
