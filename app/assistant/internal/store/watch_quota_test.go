package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWatchQuotaReservationIsAtomicAndRecoverable(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := NowMs()
	dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	hourStart := now / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	buckets := make([]DeliveryBucket, 4)
	for i := range buckets {
		var err error
		buckets[i], err = mem.UpsertDeliveryBucket(ctx, 7, int64(i+1), int64(i+1)*120_000, now)
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	allowed := make(chan int, len(buckets))
	for i, bucket := range buckets {
		wg.Add(1)
		go func(index int, bucket DeliveryBucket) {
			defer wg.Done()
			ok, _, err := mem.ReserveWatchQuota(ctx, bucket.ID, 7, []int64{11}, dayStart, hourStart, 20, 3)
			if err != nil {
				t.Errorf("reserve %d: %v", index, err)
				return
			}
			if ok {
				allowed <- index
			}
		}(i, bucket)
	}
	wg.Wait()
	close(allowed)
	allowedIndexes := make([]int, 0, 3)
	for index := range allowed {
		allowedIndexes = append(allowedIndexes, index)
	}
	if len(allowedIndexes) != 3 {
		t.Fatalf("allowed reservations=%v", allowedIndexes)
	}

	released := buckets[allowedIndexes[0]]
	if err := mem.MarkBucketScheduled(ctx, released.ID, 101); err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishWatchDelivery(ctx, released.ID, 7, 101, StatusCancelled, now); err != nil {
		t.Fatal(err)
	}
	var blocked DeliveryBucket
	for i, bucket := range buckets {
		found := false
		for _, index := range allowedIndexes {
			found = found || index == i
		}
		if !found {
			blocked = bucket
		}
	}
	ok, _, err := mem.ReserveWatchQuota(ctx, blocked.ID, 7, []int64{11}, dayStart, hourStart, 20, 3)
	if err != nil || !ok {
		t.Fatalf("released quota was not reusable: ok=%v err=%v", ok, err)
	}

	delivered := buckets[allowedIndexes[1]]
	if err := mem.MarkBucketScheduled(ctx, delivered.ID, 102); err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishWatchDelivery(ctx, delivered.ID, 7, 102, StatusDone, now); err != nil {
		t.Fatal(err)
	}
	if daily, _ := mem.CountSent(ctx, 7, 0, "day", dayStart); daily != 1 {
		t.Fatalf("daily=%d", daily)
	}
	if hourly, _ := mem.CountSent(ctx, 7, 11, "hour", hourStart); hourly != 1 {
		t.Fatalf("hourly=%d", hourly)
	}
}
