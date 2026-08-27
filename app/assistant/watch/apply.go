package watch

import (
	"context"
	"fmt"

	"esx/pkg/event"
)

// PostEventKey is the Watch execution dedupe key for a post lifecycle event (WCH-011).
func PostEventKey(ev event.PostEvent) string {
	return fmt.Sprintf("post:%d", ev.EventID)
}

// ApplyPostEvent evaluates enabled tasks against a post lifecycle event and
// records hits. Matcher failures must not affect the post write path (WCH-013).
func ApplyPostEvent(ctx context.Context, store Store, ev event.PostEvent) error {
	if store == nil {
		return nil
	}
	if err := ev.Validate(); err != nil {
		return nil
	}
	tasks, err := store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	key := PostEventKey(ev)
	for _, task := range tasks {
		ok, summary := Match(task, ev)
		if !ok {
			continue
		}
		if err := store.RecordHit(ctx, Hit{
			UserID:  task.UserID,
			TaskID:  task.ID,
			PostID:  ev.PostID,
			Title:   ev.Title,
			Summary: summary,
		}, key); err != nil {
			return err
		}
	}
	return nil
}
