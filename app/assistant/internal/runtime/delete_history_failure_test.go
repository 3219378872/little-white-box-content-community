package runtime

import (
	"context"
	"errors"
	"testing"

	"esx/app/assistant/internal/store"

	"github.com/stretchr/testify/require"
)

type failingHistoryOutbox struct {
	store.Store
	err error
}

func (s failingHistoryOutbox) Transact(ctx context.Context, fn func(context.Context, store.Store) error) error {
	return s.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		return fn(ctx, failingHistoryOutbox{Store: tx, err: s.err})
	})
}

func (s failingHistoryOutbox) InsertOutbox(context.Context, store.Outbox) error { return s.err }

func TestDeleteHistoryPropagatesOutboxFailure(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	_, err := mem.InsertMessage(ctx, store.Message{UserID: 7, Content: "private", Visible: true})
	require.NoError(t, err)
	want := errors.New("outbox unavailable")
	a := &Acceptor{Store: failingHistoryOutbox{Store: mem, err: want}}
	require.ErrorIs(t, a.DeleteHistory(ctx, 7), want)
}
