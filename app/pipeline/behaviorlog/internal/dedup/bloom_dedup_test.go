package dedup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeBloomStore struct {
	existsErr error
	addErr    error
	expireErr error
}

func (f fakeBloomStore) Exists(_ context.Context, _ string, _ uint, _ []byte) (bool, error) {
	return false, f.existsErr
}

func (f fakeBloomStore) Add(_ context.Context, _ string, _ uint, _ []byte) error {
	return f.addErr
}

func (f fakeBloomStore) Expire(_ context.Context, _ string, _ int) error {
	return f.expireErr
}

func TestBloomDedup_KeyContainsDate(t *testing.T) {
	d := NewBloomDedup(fakeBloomStore{}, 1024)

	key := d.keyForDate(time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "bf:behavior_events:20260429", key)
}

func TestBloomDedup_RedisError_ReturnsError(t *testing.T) {
	d := NewBloomDedup(fakeBloomStore{existsErr: errors.New("redis down")}, 1024)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := d.IsDuplicate(ctx, "event-unavailable")
	assert.Error(t, err)
}

func TestBloomDedup_MarkProcessed_RedisError_ReturnsError(t *testing.T) {
	d := NewBloomDedup(fakeBloomStore{addErr: errors.New("redis down")}, 1024)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := d.MarkProcessed(ctx, "event-unavailable")
	assert.Error(t, err)
}
