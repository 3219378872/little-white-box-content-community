package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"

	"google.golang.org/grpc"
)

type fakeContentService struct {
	assertFn func(ctx context.Context, in *contentservice.AssertInteractableReq) (*contentservice.AssertInteractableResp, error)
}

func (f *fakeContentService) AssertInteractable(ctx context.Context, in *contentservice.AssertInteractableReq, _ ...grpc.CallOption) (*contentservice.AssertInteractableResp, error) {
	if f.assertFn != nil {
		return f.assertFn(ctx, in)
	}
	return &contentservice.AssertInteractableResp{}, nil
}

func TestRequirePublishedPost_MissingContentFailsClosed(t *testing.T) {
	err := requirePublishedPost(context.Background(), nil, 11)
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("got %v, want ServiceUnavailable", err)
	}
}

func TestRequirePublishedPost_DraftIsNotFound(t *testing.T) {
	content := &fakeContentService{assertFn: func(_ context.Context, _ *contentservice.AssertInteractableReq) (*contentservice.AssertInteractableResp, error) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}}
	err := requirePublishedPost(context.Background(), content, 11)
	if !errx.Is(err, errx.ContentNotFound) {
		t.Fatalf("got %v, want ContentNotFound", err)
	}
}

func TestRequirePublishedPost_PublishedOK(t *testing.T) {
	if err := requirePublishedPost(context.Background(), &fakeContentService{}, 11); err != nil {
		t.Fatal(err)
	}
}

func TestRequirePublishedLikeTarget_CommentUnpublishedParent(t *testing.T) {
	content := &fakeContentService{assertFn: func(_ context.Context, in *contentservice.AssertInteractableReq) (*contentservice.AssertInteractableResp, error) {
		if in.TargetType != 2 || in.TargetId != 21 {
			t.Fatalf("unexpected target %+v", in)
		}
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}}
	err := requirePublishedLikeTarget(context.Background(), content, 21, 2)
	if !errx.Is(err, errx.ContentNotFound) {
		t.Fatalf("got %v, want ContentNotFound", err)
	}
}

func TestRequirePublishedLikeTarget_CommentOK(t *testing.T) {
	if err := requirePublishedLikeTarget(context.Background(), &fakeContentService{}, 21, 2); err != nil {
		t.Fatal(err)
	}
}
