package runtime

import (
	"context"
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
