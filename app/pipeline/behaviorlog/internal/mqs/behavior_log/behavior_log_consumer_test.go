package behaviorlog

import (
	"context"
	"errors"
	"testing"

	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	"esx/pkg/event"
	"mqx"

	rocketconsumer "github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProcessor struct {
	events []event.BehaviorEvent
	metas  []behaviorlogic.MessageMeta
	err    error
}

func (m *mockProcessor) Process(_ context.Context, e event.BehaviorEvent, meta behaviorlogic.MessageMeta) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, e)
	m.metas = append(m.metas, meta)
	return nil
}

func makeMsg(body string) *primitive.MessageExt {
	return &primitive.MessageExt{
		Message:        primitive.Message{Body: []byte(body)},
		MsgId:          "test-msg",
		StoreTimestamp: 1714300000000,
	}
}

func TestConsumeBehaviorMsg(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		processErr error
		want       rocketconsumer.ConsumeResult
		check      func(t *testing.T, processor *mockProcessor)
	}{
		{
			name: "valid event routes to processor",
			body: `{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`,
			want: rocketconsumer.ConsumeSuccess,
			check: func(t *testing.T, processor *mockProcessor) {
				require.Len(t, processor.events, 1)
				assert.Equal(t, int64(42), processor.events[0].UserID)
				assert.Equal(t, "like", processor.events[0].Action)
				assert.Equal(t, "test-msg", processor.metas[0].MsgID)
			},
		},
		{
			name: "malformed json skips without processing",
			body: `bad-json`,
			want: rocketconsumer.ConsumeSuccess,
			check: func(t *testing.T, processor *mockProcessor) {
				assert.Empty(t, processor.events)
			},
		},
		{
			name:       "processor permanent error skips",
			body:       `{"event_id":1,"event_time":1714300000000,"user_id":0,"action":"like","target_id":999,"target_type":"post"}`,
			processErr: mqx.ErrPermanentEvent("missing user_id"),
			want:       rocketconsumer.ConsumeSuccess,
			check: func(t *testing.T, processor *mockProcessor) {
				assert.Empty(t, processor.events)
			},
		},
		{
			name:       "processor transient error retries",
			body:       `{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`,
			processErr: errors.New("clickhouse down"),
			want:       rocketconsumer.ConsumeRetryLater,
		},
		{
			name: "zero event id passes message metadata to processor",
			body: `{"user_id":42,"action":"like","target_id":999,"target_type":"post"}`,
			want: rocketconsumer.ConsumeSuccess,
			check: func(t *testing.T, processor *mockProcessor) {
				require.Len(t, processor.events, 1)
				assert.Zero(t, processor.events[0].EventID)
				assert.Zero(t, processor.events[0].EventTime)
				assert.Equal(t, behaviorlogic.MessageMeta{MsgID: "test-msg", StoreTimestamp: 1714300000000}, processor.metas[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &mockProcessor{err: tt.processErr}
			result := consumeBehaviorMsg(context.Background(), processor, makeMsg(tt.body))

			assert.Equal(t, tt.want, result)
			if tt.check != nil {
				tt.check(t, processor)
			}
		})
	}
}

func TestMakeBehaviorHandler_RetriesBatchOnTransientError(t *testing.T) {
	processor := &mockProcessor{err: errors.New("clickhouse down")}
	handler := MakeBehaviorHandler(processor)

	result, err := handler(context.Background(),
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	require.NoError(t, err)
	assert.Equal(t, rocketconsumer.ConsumeRetryLater, result)
}
