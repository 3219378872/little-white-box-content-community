package watch

import (
	"context"
	"testing"

	"esx/pkg/errx"
)

func TestLookupsRejectsWatchingOwnAuthorAndPost(t *testing.T) {
	lookups := Lookups{
		Author: func(_ context.Context, userID int64) error {
			if userID <= 0 {
				return errx.NewWithCode(errx.ParamError)
			}
			return nil
		},
		Post: func(_ context.Context, postID int64) (int64, error) {
			switch postID {
			case 11:
				return 8, nil
			case 12:
				return 2, nil
			default:
				return 0, errx.NewWithCode(errx.ParamError)
			}
		},
	}
	ctx := context.Background()

	if err := lookups.Validate(ctx, Task{
		UserID: 2, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 8,
	}); err != nil {
		t.Fatalf("other author: %v", err)
	}
	if err := lookups.Validate(ctx, Task{
		UserID: 2, ConditionType: AuthorNewPost, TargetType: "author", TargetID: 2,
	}); !errx.Is(err, errx.CannotWatchSelf) {
		t.Fatalf("own author: %v", err)
	}
	if err := lookups.Validate(ctx, Task{
		UserID: 2, ConditionType: PostRevised, TargetType: "post", TargetID: 11,
	}); err != nil {
		t.Fatalf("other post: %v", err)
	}
	if err := lookups.Validate(ctx, Task{
		UserID: 2, ConditionType: PostRevised, TargetType: "post", TargetID: 12,
	}); !errx.Is(err, errx.CannotWatchSelf) {
		t.Fatalf("own post revision: %v", err)
	}
	if err := lookups.Validate(ctx, Task{
		UserID: 2, ConditionType: DiscussionSpike, TargetType: "post", TargetID: 12,
	}); err != nil {
		t.Fatalf("own discussion spike: %v", err)
	}
}
