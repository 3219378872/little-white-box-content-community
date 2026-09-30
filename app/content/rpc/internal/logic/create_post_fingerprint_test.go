package logic

import (
	"context"
	"esx/app/content/rpc/internal/model"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCreatePostFingerprintBoundaries(t *testing.T) {
	base := &pb.CreatePostReq{AuthorId: 1, Title: "title", Content: "body", Status: 1, IdempotencyKey: "same-key"}
	pairs := []struct {
		name   string
		change func(*pb.CreatePostReq, *pb.CreatePostReq)
	}{
		{"comma tag elements", func(a, b *pb.CreatePostReq) { a.Tags = []string{"a,b", "c"}; b.Tags = []string{"a", "b,c"} }},
		{"single comma tag", func(a, b *pb.CreatePostReq) { a.Tags = []string{"a,b"}; b.Tags = []string{"a", "b"} }},
		{"empty elements", func(a, b *pb.CreatePostReq) { a.Tags = []string{"", "a"}; b.Tags = []string{"a", ""} }},
		{"unicode", func(a, b *pb.CreatePostReq) {
			a.Tags = []string{"标签,😀", "尾"}
			b.Tags = []string{"标签", "😀,尾"}
		}},
		{"NUL field boundary", func(a, b *pb.CreatePostReq) { a.Title = "a\x00b"; a.Content = "c"; b.Title = "a"; b.Content = "b\x00c" }},
		{"NUL list element", func(a, b *pb.CreatePostReq) { a.Tags = []string{"a\x00b"}; b.Tags = []string{"a", "b"} }},
		{"image boundaries", func(a, b *pb.CreatePostReq) { a.Images = []string{"a", "bc"}; b.Images = []string{"ab", "c"} }},
		{"media order", func(a, b *pb.CreatePostReq) { a.MediaIds = []int64{10, 20}; b.MediaIds = []int64{20, 10} }},
		{"status", func(a, b *pb.CreatePostReq) { a.Status = 0; b.Status = 1 }},
	}
	for _, tt := range pairs {
		t.Run(tt.name, func(t *testing.T) {
			a, b := proto.Clone(base).(*pb.CreatePostReq), proto.Clone(base).(*pb.CreatePostReq)
			tt.change(a, b)
			ha, hb := createPostIdempotencyRecord(a), createPostIdempotencyRecord(b)
			require.NotEqual(t, ha.CommandHash, hb.CommandHash)
			require.False(t, hb.Matches(ha.CommandHash), "same-key distinct command must conflict")
			require.True(t, ha.Matches(createPostIdempotencyRecord(proto.Clone(a).(*pb.CreatePostReq)).CommandHash))
			require.Len(t, ha.CommandHash, 64)
		})
	}
	nilRecord := createPostIdempotencyRecord(base)
	base.Tags, base.Images, base.MediaIds = []string{}, []string{}, []int64{}
	require.Equal(t, nilRecord.CommandHash, createPostIdempotencyRecord(base).CommandHash)
}

func TestCreatePostLegacyCompatibilityFailsClosedOnAmbiguity(t *testing.T) {
	base := &pb.CreatePostReq{AuthorId: 1, Title: "title", Content: "body", Status: 1, IdempotencyKey: "same-key"}
	for _, tags := range [][]string{nil, {}, {""}, {"single"}, {"标签😀"}} {
		base.Tags = tags
		rec := createPostIdempotencyRecord(base)
		require.NotEmpty(t, rec.LegacyCommandHash)
		require.True(t, rec.Matches(rec.LegacyCommandHash))
	}
	for _, tags := range [][]string{{"a,b"}, {"a", "b"}, {"a,b", "c"}, {"a", "b,c"}, {"a\x00b"}} {
		base.Tags = tags
		rec := createPostIdempotencyRecord(base)
		require.Empty(t, rec.LegacyCommandHash)
		require.False(t, rec.Matches(idempotencyx.CommandHash("title", "body", "", "a,b", "", "1")))
	}
	base.Tags = nil
	for _, pair := range [][2]string{{"a\x00b", "c"}, {"a", "b\x00c"}} {
		base.Title, base.Content = pair[0], pair[1]
		rec := createPostIdempotencyRecord(base)
		require.Empty(t, rec.LegacyCommandHash)
		require.False(t, rec.Matches(idempotencyx.CommandHash("a\x00b", "c", "", "", "", "1")))
	}
	base.Title, base.Content = "title", "body"
	base.MediaIds = []int64{10, 20}
	require.Empty(t, createPostIdempotencyRecord(base).LegacyCommandHash)
}

type fingerprintReplayCommand struct {
	capturingPostCommand
	record idempotencyx.IdempotencyRecord
	id     int64
}

func (c *fingerprintReplayCommand) CreatePost(_ context.Context, p *model.Post, _ []string, _ []int64, _ outboxx.Event, rec idempotencyx.IdempotencyRecord) (int64, bool, error) {
	if c.id != 0 {
		if !rec.Matches(c.record.CommandHash) {
			return 0, false, idempotencyx.ErrIdempotencyConflict
		}
		return c.id, false, nil
	}
	c.record, c.id = rec, p.Id
	return p.Id, true, nil
}
func TestCreatePostTagCollisionReturnsConflictAndExactRetryReplays(t *testing.T) {
	cmd := new(fingerprintReplayCommand)
	svc := newUnitSvcCtx(new(MockPostModel), nil, nil, new(MockPostTagModel))
	svc.PostCommandModel = cmd
	logic := NewCreatePostLogic(context.Background(), svc)
	req := &pb.CreatePostReq{AuthorId: 1, Title: "title", Content: "body", Status: 1, IdempotencyKey: "same-key", Tags: []string{"a,b", "c"}}
	first, err := logic.CreatePost(req)
	require.NoError(t, err)
	second, err := logic.CreatePost(req)
	require.NoError(t, err)
	require.Equal(t, first.PostId, second.PostId)
	req.Tags = []string{"a", "b,c"}
	conflict, err := logic.CreatePost(req)
	require.Nil(t, conflict)
	require.True(t, errx.Is(err, errx.IdempotencyConflict))
}
