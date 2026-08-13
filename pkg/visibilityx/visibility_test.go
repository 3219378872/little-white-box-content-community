package visibilityx

import (
	"context"
	"errors"
	"testing"

	"errx"
	"esx/pkg/validator"
)

type fakePost struct {
	id     int64
	status int32
}

func (p fakePost) GetId() int64     { return p.id }
func (p fakePost) GetStatus() int32 { return p.status }

func TestPublishedByIDs(t *testing.T) {
	t.Parallel()

	t.Run("nil fetcher fails closed", func(t *testing.T) {
		t.Parallel()
		_, err := PublishedByIDs[fakePost](context.Background(), nil, []int64{1})
		if !errx.Is(err, errx.ServiceUnavailable) {
			t.Fatalf("expected service unavailable, got %v", err)
		}
	})

	t.Run("fetch error fails closed", func(t *testing.T) {
		t.Parallel()
		want := errors.New("content down")
		_, err := PublishedByIDs(context.Background(), func(context.Context, []int64) ([]fakePost, error) {
			return nil, want
		}, []int64{1})
		if !errors.Is(err, want) {
			t.Fatalf("expected fetch error, got %v", err)
		}
	})

	t.Run("nil batch fails closed", func(t *testing.T) {
		t.Parallel()
		_, err := PublishedByIDs(context.Background(), func(context.Context, []int64) ([]fakePost, error) {
			return nil, nil
		}, []int64{1})
		if !errx.Is(err, errx.ServiceUnavailable) {
			t.Fatalf("expected service unavailable, got %v", err)
		}
	})

	t.Run("empty ids skip fetch", func(t *testing.T) {
		t.Parallel()
		called := false
		got, err := PublishedByIDs(context.Background(), func(context.Context, []int64) ([]fakePost, error) {
			called = true
			return []fakePost{}, nil
		}, []int64{0, -1})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if called {
			t.Fatal("fetcher should not be called for empty unique ids")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty map, got %#v", got)
		}
	})

	t.Run("keeps published and requested ids only", func(t *testing.T) {
		t.Parallel()
		got, err := PublishedByIDs(context.Background(), func(_ context.Context, ids []int64) ([]fakePost, error) {
			return []fakePost{
				{id: 1, status: PublishedStatus},
				{id: 2, status: 0},
				{id: 3, status: 2},
				{id: 99, status: PublishedStatus},
			}, nil
		}, []int64{1, 1, 2, 3, 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 1 || got[1].GetId() != 1 {
			t.Fatalf("expected only published id 1, got %#v", got)
		}
	})

	t.Run("batches over max query size", func(t *testing.T) {
		t.Parallel()
		ids := make([]int64, validator.MaxBatchQueryIds+3)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		var batches []int
		got, err := PublishedByIDs(context.Background(), func(_ context.Context, batch []int64) ([]fakePost, error) {
			batches = append(batches, len(batch))
			out := make([]fakePost, 0, len(batch))
			for _, id := range batch {
				out = append(out, fakePost{id: id, status: PublishedStatus})
			}
			return out, nil
		}, ids)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != len(ids) {
			t.Fatalf("expected %d posts, got %d", len(ids), len(got))
		}
		if len(batches) != 2 || batches[0] != validator.MaxBatchQueryIds || batches[1] != 3 {
			t.Fatalf("unexpected batches: %#v", batches)
		}
	})
}

func TestAdjustPageTotal(t *testing.T) {
	t.Parallel()
	if got := AdjustPageTotal(10, 3, 3); got != 10 {
		t.Fatalf("no removal: got %d", got)
	}
	if got := AdjustPageTotal(10, 3, 1); got != 8 {
		t.Fatalf("page subtract: got %d", got)
	}
	if got := AdjustPageTotal(1, 3, 1); got != 1 {
		t.Fatalf("total below removed: got %d", got)
	}
}

func TestIsPublished(t *testing.T) {
	t.Parallel()
	if !IsPublished(PublishedStatus) || IsPublished(0) || IsPublished(2) {
		t.Fatal("published status mapping is wrong")
	}
}
