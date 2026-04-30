package logic

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/event"
	"mqx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func validBehaviorEvent() event.BehaviorEvent {
	return event.BehaviorEvent{
		EventID:    1,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
	}
}

func TestRecorderProcess(t *testing.T) {
	storeErr := errors.New("clickhouse down")
	dedupErr := errors.New("redis down")
	markErr := errors.New("redis write failed")

	tests := []struct {
		name       string
		event      event.BehaviorEvent
		meta       MessageMeta
		setupMocks func(*MockBehaviorStore, *MockBehaviorDedup)
		wantErr    bool
		permanent  bool
	}{
		{
			name:  "valid event inserts and marks processed",
			event: validBehaviorEvent(),
			setupMocks: func(store *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, "1").Return(false, nil).Once()
				store.On("Insert", mock.Anything, validBehaviorEvent()).Return(nil).Once()
				dedup.On("MarkProcessed", mock.Anything, "1").Return(nil).Once()
			},
		},
		{
			name:      "invalid event returns permanent event",
			event:     event.BehaviorEvent{},
			meta:      MessageMeta{MsgID: "invalid-msg", StoreTimestamp: 1714300000000},
			wantErr:   true,
			permanent: true,
			setupMocks: func(_ *MockBehaviorStore, _ *MockBehaviorDedup) {
			},
		},
		{
			name:  "duplicate event skips store",
			event: validBehaviorEvent(),
			setupMocks: func(_ *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, "1").Return(true, nil).Once()
			},
		},
		{
			name:    "store error does not mark processed",
			event:   validBehaviorEvent(),
			wantErr: true,
			setupMocks: func(store *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, "1").Return(false, nil).Once()
				store.On("Insert", mock.Anything, validBehaviorEvent()).Return(storeErr).Once()
			},
		},
		{
			name:  "dedup error falls through to store",
			event: validBehaviorEvent(),
			setupMocks: func(store *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, "1").Return(false, dedupErr).Once()
				store.On("Insert", mock.Anything, validBehaviorEvent()).Return(nil).Once()
				dedup.On("MarkProcessed", mock.Anything, "1").Return(nil).Once()
			},
		},
		{
			name:  "mark processed error does not fail insert",
			event: validBehaviorEvent(),
			setupMocks: func(store *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, "1").Return(false, nil).Once()
				store.On("Insert", mock.Anything, validBehaviorEvent()).Return(nil).Once()
				dedup.On("MarkProcessed", mock.Anything, "1").Return(markErr).Once()
			},
		},
		{
			name:  "zero event id uses stable message id",
			event: event.BehaviorEvent{UserID: 42, Action: "like", TargetID: 999, TargetType: "post"},
			meta:  MessageMeta{MsgID: "stable-msg", StoreTimestamp: 1714300000000},
			setupMocks: func(store *MockBehaviorStore, dedup *MockBehaviorDedup) {
				dedup.On("IsDuplicate", mock.Anything, mock.AnythingOfType("string")).Return(false, nil).Once()
				store.On("Insert", mock.Anything, mock.MatchedBy(func(e event.BehaviorEvent) bool {
					return e.EventID != 0 && e.EventTime == 1714300000000
				})).Return(nil).Once()
				dedup.On("MarkProcessed", mock.Anything, mock.AnythingOfType("string")).Return(nil).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &MockBehaviorStore{}
			dedup := &MockBehaviorDedup{}
			tt.setupMocks(store, dedup)
			recorder := NewRecorder(store, dedup)

			err := recorder.Process(context.Background(), tt.event, tt.meta)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.permanent, mqx.IsPermanentEvent(err))
			} else {
				require.NoError(t, err)
			}
			store.AssertExpectations(t)
			dedup.AssertExpectations(t)
		})
	}
}
