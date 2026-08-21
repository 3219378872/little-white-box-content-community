package viewerstate

import (
	"context"
	"errors"
	"testing"

	"esx/app/gateway/internal/svc"
	"esx/app/interaction/rpc/interactionservice"

	"google.golang.org/grpc"
)

type fakeInteraction struct {
	interactionservice.InteractionService
	liked        map[int64]bool
	favorited    map[int64]bool
	likedErr     error
	favoritedErr error
}

func (f *fakeInteraction) BatchCheckLiked(_ context.Context, in *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
	if f.likedErr != nil {
		return nil, f.likedErr
	}
	results := make(map[int64]bool, len(in.TargetIds))
	for _, id := range in.TargetIds {
		if f.liked[id] {
			results[id] = true
		}
	}
	return &interactionservice.BatchCheckLikedResp{Results: results}, nil
}

func (f *fakeInteraction) BatchCheckFavorited(_ context.Context, in *interactionservice.BatchCheckFavoritedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckFavoritedResp, error) {
	if f.favoritedErr != nil {
		return nil, f.favoritedErr
	}
	results := make(map[int64]bool, len(in.PostIds))
	for _, id := range in.PostIds {
		if f.favorited[id] {
			results[id] = true
		}
	}
	return &interactionservice.BatchCheckFavoritedResp{Results: results}, nil
}

func TestEnrichFillsViewerState(t *testing.T) {
	svcCtx := &svc.ServiceContext{InteractionService: &fakeInteraction{
		liked:     map[int64]bool{1: true},
		favorited: map[int64]bool{2: true},
	}}
	liked, favorited, err := Enrich(context.Background(), svcCtx, 42, []int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if !liked[1] || liked[2] {
		t.Fatalf("unexpected liked: %v", liked)
	}
	if favorited[1] || !favorited[2] {
		t.Fatalf("unexpected favorited: %v", favorited)
	}
}

func TestEnrichAnonymousReturnsEmpty(t *testing.T) {
	liked, favorited, err := Enrich(context.Background(), &svc.ServiceContext{}, 0, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(liked) != 0 || len(favorited) != 0 {
		t.Fatalf("anonymous enrichment must be empty, got %v %v", liked, favorited)
	}
}

func TestEnrichNilServiceReturnsEmpty(t *testing.T) {
	liked, favorited, err := Enrich(context.Background(), &svc.ServiceContext{}, 42, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(liked) != 0 || len(favorited) != 0 {
		t.Fatalf("nil interaction service must degrade to empty, got %v %v", liked, favorited)
	}
}

func TestEnrichPropagatesInteractionError(t *testing.T) {
	svcCtx := &svc.ServiceContext{InteractionService: &fakeInteraction{
		likedErr: errors.New("interaction down"),
	}}
	_, _, err := Enrich(context.Background(), svcCtx, 42, []int64{1})
	if err == nil {
		t.Fatal("expected error from interaction service")
	}
}
