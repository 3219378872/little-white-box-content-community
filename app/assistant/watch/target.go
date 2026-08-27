package watch

import (
	"context"
	"strings"

	"esx/pkg/errx"
)

// Lookups validates Watch create targets (WCH-004). Nil callbacks mean the
// corresponding downstream is unavailable.
type Lookups struct {
	Author func(ctx context.Context, userID int64) error
	Post   func(ctx context.Context, postID int64) error
	Tag    func(ctx context.Context, name string) error
}

func (l Lookups) Validate(ctx context.Context, task Task) error {
	if err := ValidateTask(task); err != nil {
		return err
	}
	switch task.ConditionType {
	case AuthorNewPost:
		if l.Author == nil {
			return errx.NewWithCode(errx.ServiceUnavailable)
		}
		return l.Author(ctx, task.TargetID)
	case PostRevised, DiscussionSpike:
		if l.Post == nil {
			return errx.NewWithCode(errx.ServiceUnavailable)
		}
		return l.Post(ctx, task.TargetID)
	case TagNewPost:
		if l.Tag == nil {
			return errx.NewWithCode(errx.ServiceUnavailable)
		}
		return l.Tag(ctx, strings.TrimSpace(task.TargetText))
	case KeywordNewPost:
		return nil
	default:
		return errx.New(errx.ParamError, "unknown watch condition")
	}
}
