package logic

import (
	"context"
	"testing"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
)

type replayPostCommand struct {
	replayResult int64
	replayCalls  int
	mutations    int
}

func (m *replayPostCommand) CreatePost(context.Context, *model.Post, []string, []int64, outboxx.Event, idempotencyx.IdempotencyRecord) (int64, bool, error) {
	panic("unexpected create")
}

func (m *replayPostCommand) UpdatePost(context.Context, int64, map[string]any, []string, []int64, outboxx.Event, int64, bool) error {
	m.mutations++
	return nil
}

func (m *replayPostCommand) DeletePost(context.Context, int64, outboxx.Event, int64) error {
	m.mutations++
	return nil
}

func (m *replayPostCommand) ReplayPostCommand(context.Context, idempotencyx.IdempotencyRecord) (int64, bool, error) {
	m.replayCalls++
	return m.replayResult, true, nil
}

func (m *replayPostCommand) UpdatePostIdempotent(context.Context, int64, map[string]any, []string, []int64, outboxx.Event, int64, bool, int64, idempotencyx.IdempotencyRecord) (bool, error) {
	m.mutations++
	return true, nil
}

func (m *replayPostCommand) DeletePostIdempotent(context.Context, int64, outboxx.Event, int64, int64, idempotencyx.IdempotencyRecord) (bool, error) {
	m.mutations++
	return true, nil
}

func TestUpdatePostIdempotencyReplayPrecedesRevisionRead(t *testing.T) {
	command := &replayPostCommand{replayResult: encodePostCommandResult(1, 8)}
	logic := NewUpdatePostLogic(context.Background(), &svc.ServiceContext{PostCommandModel: command})
	resp, err := logic.UpdatePost(&pb.UpdatePostReq{
		PostId: 9, AuthorId: 3, Title: "after", ExpectedRevision: 7, IdempotencyKey: "agent:update:r:c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 1 || resp.Revision != 8 || command.replayCalls != 1 || command.mutations != 0 {
		t.Fatalf("resp=%+v replay=%d mutations=%d", resp, command.replayCalls, command.mutations)
	}
}

func TestDeletePostIdempotencyReplayPrecedesDeletedRevisionRead(t *testing.T) {
	command := &replayPostCommand{replayResult: 8}
	logic := NewDeletePostLogic(context.Background(), &svc.ServiceContext{PostCommandModel: command})
	resp, err := logic.DeletePost(&pb.DeletePostReq{
		PostId: 9, AuthorId: 3, ExpectedRevision: 7, IdempotencyKey: "agent:delete:r:c",
	})
	if err != nil || resp == nil || command.replayCalls != 1 || command.mutations != 0 {
		t.Fatalf("resp=%+v err=%v replay=%d mutations=%d", resp, err, command.replayCalls, command.mutations)
	}
}
