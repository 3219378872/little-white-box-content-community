package tool

import (
	"context"
	"encoding/json"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type revisionContent struct {
	contentservice.ContentService
	userID   int64
	postID   int64
	revision int64
}

func (c *revisionContent) GetPost(
	_ context.Context,
	_ *contentservice.GetPostReq,
	_ ...grpc.CallOption,
) (*contentservice.GetPostResp, error) {
	return &contentservice.GetPostResp{Post: &contentservice.PostInfo{
		Id: c.postID, AuthorId: c.userID, Revision: c.revision, Status: 1,
	}}, nil
}

func TestPostRevisionIsFrozenBeforeCanonicalDigest(t *testing.T) {
	content := &revisionContent{userID: 3, postID: 9, revision: 7}
	registry, err := NewRegistry(Clients{Content: content}, []string{DeletePost, UpdatePost})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{UserID: 3}
	prepared, err := registry.Prepare(context.Background(), session, DeletePost, `{"post_id":9}`)
	if err != nil {
		t.Fatal(err)
	}
	var args struct {
		PostID           int64 `json:"post_id"`
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := json.Unmarshal([]byte(prepared), &args); err != nil {
		t.Fatal(err)
	}
	if args.PostID != 9 || args.ExpectedRevision != 7 {
		t.Fatalf("prepared args=%s", prepared)
	}
	firstDigest, err := CanonicalDigest(prepared)
	if err != nil || firstDigest == "" {
		t.Fatalf("digest=%q err=%v", firstDigest, err)
	}
	content.revision = 8
	if _, err := registry.Prepare(context.Background(), session, DeletePost, prepared); !errx.Is(err, errx.ContentVersionConflict) {
		t.Fatalf("changed revision err=%v", err)
	}
}
