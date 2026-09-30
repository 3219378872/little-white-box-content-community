package logic

import (
	"context"
	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/pkg/event"
	"testing"
)

type exposureExactKeys struct{ keys map[string]string }

func (s *exposureExactKeys) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.keys[key]
	return ok, nil
}
func (s *exposureExactKeys) Set(_ context.Context, key, value string, _ int) error {
	s.keys[key] = value
	return nil
}

type exposureAnalyticStore struct{ events []event.BehaviorEvent }

func (s *exposureAnalyticStore) Insert(_ context.Context, e event.BehaviorEvent) error {
	s.events = append(s.events, e)
	return nil
}
func TestExposureBusinessKeyProducesOneAnalyticEffect(t *testing.T) {
	keys := &exposureExactKeys{map[string]string{}}
	store := &exposureAnalyticStore{}
	recorder := NewRecorder(store, dedup.NewExactDedup(keys, 7776000))
	first := validBehaviorEvent()
	first.Action = event.BehaviorActionExposure
	first.RequestID = "request-1"
	first.Scene = "home"
	first.Position = new(int32(1))
	first.EventID = event.DeterministicBehaviorEventID("audit-first")
	first.ClientEventID = "audit-first"
	second := first
	second.EventID = event.DeterministicBehaviorEventID("audit-second")
	second.ClientEventID = "audit-second"
	for _, e := range []event.BehaviorEvent{first, second, first, second} {
		if err := recorder.Process(context.Background(), e, MessageMeta{}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("real Recorder+ExactDedup with in-memory key primitive: inserts=%d keys=%v; two same request/post exposures each replayed once", len(store.events), keys.keys)
	if got := len(store.events); got != 1 {
		t.Errorf("REL-004: same(requestId,postId) inserted%d times, want1", got)
	}
}
