package behaviorlog

import (
	"context"
	"errors"
	"testing"

	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"esx/pkg/mqx"

	rocketconsumer "github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validEventJSON = `{"event_id":1,"client_event_id":"client-1","schema_version":2,"event_time":1714300000000,"received_at":1714300000100,"user_id":42,"action":"like","target_id":999,"target_type":"post","producer":"behavior-rpc"}`

type mockProcessor struct {
	events []event.BehaviorEvent
	metas  []behaviorlogic.MessageMeta
	err    error
}

func (m *mockProcessor) Process(_ context.Context, behavior event.BehaviorEvent, meta behaviorlogic.MessageMeta) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, behavior)
	m.metas = append(m.metas, meta)
	return nil
}

type fakeDeadLetters struct {
	letters []store.DeadLetter
	err     error
}

func (d *fakeDeadLetters) InsertDeadLetter(_ context.Context, letter store.DeadLetter) error {
	if d.err != nil {
		return d.err
	}
	d.letters = append(d.letters, letter)
	return nil
}

func makeMsg(body string) *primitive.MessageExt {
	return &primitive.MessageExt{
		Body: []byte(body), MsgId: "test-msg",
		StoreTimestamp: 1714300000000,
	}
}

func TestConsumeBehaviorMsg(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		processErr error
		want       rocketconsumer.ConsumeResult
		deadCount  int
	}{
		{name: "valid event routes to processor", body: validEventJSON, want: rocketconsumer.ConsumeSuccess},
		{name: "malformed json records dead letter", body: `bad-json`, want: rocketconsumer.ConsumeSuccess, deadCount: 1},
		{name: "processor permanent error records dead letter", body: validEventJSON, processErr: mqx.ErrPermanentEvent("invalid event"), want: rocketconsumer.ConsumeSuccess, deadCount: 1},
		{name: "processor transient error retries", body: validEventJSON, processErr: errors.New("clickhouse down"), want: rocketconsumer.ConsumeRetryLater},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &mockProcessor{err: tt.processErr}
			deadLetters := &fakeDeadLetters{}
			result := consumeBehaviorMsg(context.Background(), processor, deadLetters, makeMsg(tt.body))

			assert.Equal(t, tt.want, result)
			assert.Len(t, deadLetters.letters, tt.deadCount)
			if tt.processErr == nil && tt.deadCount == 0 {
				require.Len(t, processor.events, 1)
				assert.Equal(t, int64(42), processor.events[0].UserID)
				assert.Equal(t, "test-msg", processor.metas[0].MsgID)
			}
		})
	}
}

func TestConsumeBehaviorMsgRetriesWhenDeadLetterWriteFails(t *testing.T) {
	result := consumeBehaviorMsg(context.Background(), &mockProcessor{},
		&fakeDeadLetters{err: errors.New("clickhouse down")}, makeMsg("bad-json"))
	assert.Equal(t, rocketconsumer.ConsumeRetryLater, result)
}

func TestMakeBehaviorHandlerRetriesBatchOnTransientError(t *testing.T) {
	processor := &mockProcessor{err: errors.New("clickhouse down")}
	handler := MakeBehaviorHandler(processor, &fakeDeadLetters{})

	result, err := handler(context.Background(), makeMsg(validEventJSON))

	require.NoError(t, err)
	assert.Equal(t, rocketconsumer.ConsumeRetryLater, result)
}
