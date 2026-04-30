package logic

import (
	"context"

	"esx/pkg/event"

	"github.com/stretchr/testify/mock"
)

type MockBehaviorStore struct {
	mock.Mock
}

func (m *MockBehaviorStore) Insert(ctx context.Context, e event.BehaviorEvent) error {
	args := m.Called(ctx, e)
	return args.Error(0)
}

type MockBehaviorDedup struct {
	mock.Mock
}

func (m *MockBehaviorDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	args := m.Called(ctx, eventID)
	return args.Bool(0), args.Error(1)
}

func (m *MockBehaviorDedup) MarkProcessed(ctx context.Context, eventID string) error {
	args := m.Called(ctx, eventID)
	return args.Error(0)
}
