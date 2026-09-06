package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
	"fmt"
	"strings"
)

func listWatchTasksExecutor(w watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, _ string) (string, []store.SourceRef, error) {
		if w == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		tasks, err := w.ListTasks(ctx, session.UserID)
		if err != nil {
			return "", nil, err
		}
		if len(tasks) == 0 {
			return "目前没有追踪任务。", nil, nil
		}
		var b strings.Builder
		for _, task := range tasks {
			fmt.Fprintf(&b, "- id=%d %s %s/%d [%v]\n", task.ID, task.ConditionType, task.TargetType, task.TargetID, task.Enabled)
		}
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}

func createWatchTaskExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Watch == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ConditionType string `json:"condition_type"`
			TargetType    string `json:"target_type"`
			TargetID      int64  `json:"target_id"`
			TargetText    string `json:"target_text"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "create_watch_task arguments are invalid")
		}
		task := watch.Task{UserID: session.UserID, ConditionType: args.ConditionType, TargetType: args.TargetType, TargetID: args.TargetID, TargetText: args.TargetText}
		if err := WatchLookups(clients).Validate(ctx, task); err != nil {
			return "", nil, err
		}
		created, err := clients.Watch.Create(ctx, task)
		if err != nil {
			if session.Recovery && errx.Is(err, errx.IdempotencyConflict) {
				tasks, listErr := clients.Watch.ListTasks(ctx, session.UserID)
				if listErr != nil {
					return "", nil, listErr
				}
				for _, existing := range tasks {
					if existing.ConditionType == task.ConditionType && existing.TargetType == task.TargetType &&
						existing.TargetID == task.TargetID && strings.TrimSpace(existing.TargetText) == strings.TrimSpace(task.TargetText) {
						return fmt.Sprintf("已创建追踪 #%d（%s）。", existing.ID, existing.ConditionType), nil, nil
					}
				}
			}
			return "", nil, err
		}
		return fmt.Sprintf("已创建追踪 #%d（%s）。", created.ID, created.ConditionType), nil, nil
	}
}

func WatchLookups(clients Clients) watch.Lookups {
	return watch.Lookups{
		Author: func(ctx context.Context, userID int64) error {
			if clients.User == nil {
				return errx.NewWithCode(errx.ServiceUnavailable)
			}
			resp, err := clients.User.GetUser(ctx, &userservice.GetUserReq{UserId: userID})
			if err != nil {
				return errx.FromRPCError(err)
			}
			if resp == nil || resp.User == nil {
				return errx.New(errx.ParamError, "watch author does not exist")
			}
			return nil
		},
		Post: func(ctx context.Context, postID int64) error {
			if clients.Content == nil {
				return errx.NewWithCode(errx.ServiceUnavailable)
			}
			resp, err := clients.Content.GetPost(ctx, &contentservice.GetPostReq{PostId: postID})
			if err != nil {
				return errx.FromRPCError(err)
			}
			if resp == nil || resp.Post == nil || !visibilityx.IsPublished(resp.Post.Status) {
				return errx.New(errx.ParamError, "watch post is not published")
			}
			return nil
		},
		Tag: func(ctx context.Context, name string) error {
			name = strings.TrimSpace(name)
			if name == "" {
				return errx.New(errx.ParamError, "watch target_text is required")
			}
			if clients.Search != nil {
				resp, err := clients.Search.SearchTags(ctx, &searchservice.SearchTagsReq{Keyword: name, Limit: 20})
				if err != nil {
					return errx.FromRPCError(err)
				}
				for _, tag := range resp.GetTags() {
					if tag != nil && strings.EqualFold(tag.Name, name) {
						return nil
					}
				}
			}
			return errx.New(errx.ParamError, "watch tag does not exist")
		},
	}
}

func updateWatchTaskExecutor(w watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if w == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID              int64 `json:"id"`
			Enabled         bool  `json:"enabled"`
			ExpectedVersion int32 `json:"expected_version"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.ID <= 0 || args.ExpectedVersion <= 0 {
			return "", nil, errx.New(errx.ParamError, "update_watch_task arguments are invalid")
		}
		updated, err := w.UpdateEnabled(ctx, session.UserID, args.ID, args.Enabled, args.ExpectedVersion)
		if err != nil {
			if session.Recovery && errx.Is(err, errx.ContentVersionConflict) {
				current, getErr := w.GetTask(ctx, session.UserID, args.ID)
				if getErr != nil {
					return "", nil, getErr
				}
				if current.Version == args.ExpectedVersion+1 && current.Enabled == args.Enabled {
					return fmt.Sprintf("追踪已更新: id=%d version=%d。", args.ID, current.Version), nil, nil
				}
			}
			return "", nil, err
		}
		return fmt.Sprintf("追踪已更新: id=%d version=%d。", args.ID, updated.Version), nil, nil
	}
}

func deleteWatchTaskExecutor(w watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if w == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID              int64 `json:"id"`
			ExpectedVersion int32 `json:"expected_version"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.ID <= 0 || args.ExpectedVersion <= 0 {
			return "", nil, errx.New(errx.ParamError, "delete_watch_task arguments are invalid")
		}
		if err := w.Delete(ctx, session.UserID, args.ID, args.ExpectedVersion); err != nil {
			if session.Recovery && errx.Is(err, errx.NotFound) {
				return "追踪已删除。", nil, nil
			}
			return "", nil, err
		}
		return "追踪已删除。", nil, nil
	}
}
