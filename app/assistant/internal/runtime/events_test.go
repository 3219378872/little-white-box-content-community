package runtime

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

type blockingWakeNotifier struct{}

func (blockingWakeNotifier) Wake(context.Context, int64) error { return nil }
func (blockingWakeNotifier) WakeToken(ctx context.Context, _ int64) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestSubscribePollsMySQLWithoutWaitingForRedis(t *testing.T) {
	original := subscribePollInterval
	subscribePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { subscribePollInterval = original })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mem := store.NewMemoryStore()
	run, err := mem.InsertRun(ctx, store.Run{UserID: 7, Status: store.StatusRunning})
	if err != nil {
		t.Fatal(err)
	}
	emitted := make(chan *pb.RunEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- Subscribe(ctx, mem, blockingWakeNotifier{}, 7, run.ID, 0, func(event *pb.RunEvent) error {
			emitted <- event
			return nil
		})
	}()

	if _, err := mem.InsertEvent(ctx, run.ID, store.EventToken, []byte(`{"text":"persisted"}`), store.NowMs()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-emitted:
		if event.Text != "persisted" {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("event persisted without a Redis wake was not polled")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not stop")
	}
}

// Finish between the event read and terminal-state read to reproduce the
// subscription's final-drain race without timing-dependent sleeps.
type finishBetweenEventReads struct {
	store.Store
	run       store.Run
	reads     int
	eventType string
	drainErr  error
}

func (s *finishBetweenEventReads) ListEventsAfter(ctx context.Context, runID, after int64) ([]store.Event, error) {
	s.reads++
	if s.reads == 3 && s.drainErr != nil {
		return nil, s.drainErr
	}
	events, err := s.Store.ListEventsAfter(ctx, runID, after)
	if err != nil {
		return nil, err
	}
	if s.reads == 2 {
		s.run.Status = store.StatusDone
		if s.eventType == store.EventError {
			s.run.Status = store.StatusError
		}
		if err := s.Store.UpdateRun(ctx, s.run); err != nil {
			return nil, err
		}
		if _, err := s.Store.InsertEvent(ctx, runID, s.eventType, []byte(`{"text":"final result"}`), store.NowMs()); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func TestSubscribeDrainsTerminalCommittedBetweenReads(t *testing.T) {
	original := subscribePollInterval
	subscribePollInterval = time.Millisecond
	t.Cleanup(func() { subscribePollInterval = original })
	for _, eventType := range []string{store.EventDone, store.EventError} {
		t.Run(eventType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			st := store.NewMemoryStore()
			run, err := st.InsertRun(ctx, store.Run{UserID: 7, Status: store.StatusRunning})
			require.NoError(t, err)
			wrapped := &finishBetweenEventReads{Store: st, run: run, eventType: eventType}
			var got []*pb.RunEvent
			require.NoError(t, Subscribe(ctx, wrapped, nil, 7, run.ID, 0, func(ev *pb.RunEvent) error { got = append(got, ev); return nil }))
			require.Len(t, got, 1)
			require.Equal(t, eventType, got[0].Type)
			require.Equal(t, "final result", got[0].Text)
		})
	}
}

func TestSubscribePropagatesFinalDrainFailure(t *testing.T) {
	original := subscribePollInterval
	subscribePollInterval = time.Millisecond
	t.Cleanup(func() { subscribePollInterval = original })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	st := store.NewMemoryStore()
	run, err := st.InsertRun(ctx, store.Run{UserID: 7, Status: store.StatusRunning})
	require.NoError(t, err)
	want := errors.New("final read unavailable")
	wrapped := &finishBetweenEventReads{Store: st, run: run, eventType: store.EventDone, drainErr: want}
	require.ErrorIs(t, Subscribe(ctx, wrapped, nil, 7, run.ID, 0, func(*pb.RunEvent) error { return nil }), want)
}
