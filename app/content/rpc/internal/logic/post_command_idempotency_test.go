package logic

import (
	"context"
	"encoding/json"
	"testing"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type replayPostCommand struct {
	replayResult int64
	replayHash   string
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

func (m *replayPostCommand) ReplayPostCommand(_ context.Context, record idempotencyx.IdempotencyRecord) (int64, bool, error) {
	m.replayCalls++
	if m.replayHash != "" && m.replayHash != record.CommandHash {
		return 0, false, idempotencyx.ErrIdempotencyConflict
	}
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

func TestUpdatePostIdempotencyPreservesExistingCommandHashes(t *testing.T) {
	for _, test := range []struct {
		name    string
		request *pb.UpdatePostReq
	}{
		{"title only", &pb.UpdatePostReq{Title: "after"}},
		{"existing images", &pb.UpdatePostReq{Images: []string{"owned.jpg"}, ImagesProvided: true}},
		{"existing media IDs", &pb.UpdatePostReq{MediaIds: []int64{10}, MediaIdsProvided: true}},
		{"existing images and media IDs", &pb.UpdatePostReq{Images: []string{"owned.jpg"}, ImagesProvided: true, MediaIds: []int64{10}, MediaIdsProvided: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := test.request
			request.PostId, request.AuthorId, request.ExpectedRevision = 9, 3, 7
			request.IdempotencyKey = "agent:update:r:c"
			legacyHash := legacyUpdatePostHash(t, request)
			record, err := updatePostIdempotency(request)
			require.NoError(t, err)
			require.Equal(t, legacyHash, record.CommandHash)

			command := &replayPostCommand{replayHash: legacyHash, replayResult: encodePostCommandResult(1, 8)}
			response, err := NewUpdatePostLogic(context.Background(), &svc.ServiceContext{PostCommandModel: command}).UpdatePost(request)
			require.NoError(t, err)
			require.Equal(t, int64(8), response.Revision)
			require.Equal(t, 1, command.replayCalls)
			require.Zero(t, command.mutations)
		})
	}
}

func TestUpdatePostExplicitEmptyMediaConflictsWithOmittedReplay(t *testing.T) {
	base := &pb.UpdatePostReq{PostId: 9, AuthorId: 3, Title: "after", ExpectedRevision: 7, IdempotencyKey: "agent:update:r:c"}
	for _, field := range []string{"images", "media IDs"} {
		t.Run(field, func(t *testing.T) {
			request := proto.Clone(base).(*pb.UpdatePostReq)
			if field == "images" {
				request.ImagesProvided, request.Images = true, []string{}
			} else {
				request.MediaIdsProvided, request.MediaIds = true, []int64{}
			}
			wire, err := proto.Marshal(request)
			require.NoError(t, err)
			request = new(pb.UpdatePostReq)
			require.NoError(t, proto.Unmarshal(wire, request))
			command := &replayPostCommand{replayHash: legacyUpdatePostHash(t, base), replayResult: encodePostCommandResult(1, 8)}
			response, err := NewUpdatePostLogic(context.Background(), &svc.ServiceContext{PostCommandModel: command}).UpdatePost(request)
			require.Nil(t, response)
			require.True(t, errx.Is(err, errx.IdempotencyConflict))
			require.Equal(t, 1, command.replayCalls)
			require.Zero(t, command.mutations)
		})
	}
}

func legacyUpdatePostHash(t *testing.T, in *pb.UpdatePostReq) string {
	t.Helper()
	payload, err := json.Marshal(struct {
		PostID           int64    `json:"post_id"`
		AuthorID         int64    `json:"author_id"`
		Title            string   `json:"title"`
		Content          string   `json:"content"`
		Images           []string `json:"images"`
		Tags             []string `json:"tags"`
		Status           *int32   `json:"status"`
		ExpectedRevision int64    `json:"expected_revision"`
		MediaIDs         []int64  `json:"media_ids"`
	}{in.PostId, in.AuthorId, in.Title, in.Content, in.Images, in.Tags, in.Status, in.ExpectedRevision, in.MediaIds})
	require.NoError(t, err)
	return idempotencyx.CommandHash(string(payload))
}
