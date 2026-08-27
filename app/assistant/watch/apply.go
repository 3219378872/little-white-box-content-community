package watch

import (
	"context"
	"fmt"
	"strings"

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

const DefaultSpikeMinComments = 5

// SpikeJudge is the optional LLM step after discussion_spike prefilter.
// Nil means no model is configured: at/above threshold records failed/skipped
// and must not be rewritten as a rule hit (WCH-012).
type SpikeJudge func(ctx context.Context, task Task, postID int64, count int) (bool, error)

type SpikeOptions struct {
	MinComments int
	Judge       SpikeJudge
}

func BehaviorEventKey(ev event.BehaviorEvent) string {
	return fmt.Sprintf("behavior:%d", ev.EventID)
}

func SpikeEventKey(postID, eventID int64) string {
	return fmt.Sprintf("spike:%d:%d", postID, eventID)
}

func SpikeEventPrefix(postID int64) string {
	return fmt.Sprintf("spike:%d:", postID)
}

// ApplyBehaviorEvent handles user-behavior-v2 for discussion_spike (WCH-003/012).
func ApplyBehaviorEvent(ctx context.Context, store Store, ev event.BehaviorEvent, opts SpikeOptions) error {
	if store == nil {
		return nil
	}
	if err := ev.Validate(); err != nil {
		return nil
	}
	if ev.Action != event.BehaviorActionComment || !strings.EqualFold(strings.TrimSpace(ev.TargetType), "post") || ev.TargetID <= 0 {
		return nil
	}
	minComments := opts.MinComments
	if minComments <= 0 {
		minComments = DefaultSpikeMinComments
	}
	tasks, err := store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ConditionType != DiscussionSpike || task.TargetID != ev.TargetID {
			continue
		}
		prefix := SpikeEventPrefix(ev.TargetID)
		prior, err := store.CountExecutions(ctx, task.ID, prefix)
		if err != nil {
			return err
		}
		count := int(prior) + 1
		eventKey := SpikeEventKey(ev.TargetID, ev.EventID)
		if count < minComments {
			if err := store.RecordExecution(ctx, task.ID, eventKey, "skipped", false); err != nil {
				return err
			}
			continue
		}
		if opts.Judge == nil {
			if err := store.RecordExecution(ctx, task.ID, eventKey, "failed", false); err != nil {
				return err
			}
			continue
		}
		hit, judgeErr := opts.Judge(ctx, task, ev.TargetID, count)
		if judgeErr != nil {
			if err := store.RecordExecution(ctx, task.ID, eventKey, "failed", true); err != nil {
				return err
			}
			continue
		}
		if !hit {
			if err := store.RecordExecution(ctx, task.ID, eventKey, "skipped", true); err != nil {
				return err
			}
			continue
		}
		if err := store.RecordHit(ctx, Hit{
			UserID:  task.UserID,
			TaskID:  task.ID,
			PostID:  ev.TargetID,
			Summary: "讨论量明显上升",
		}, eventKey); err != nil {
			return err
		}
	}
	return nil
}
