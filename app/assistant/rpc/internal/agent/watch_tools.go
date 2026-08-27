package agent

import (
	"context"
	"fmt"
	"strings"

	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/watch"
	"esx/pkg/errx"
)

const (
	ToolCreateWatchTask = "create_watch_task"
	ToolListWatchTasks  = "list_watch_tasks"
	ToolUpdateWatchTask = "update_watch_task"
	ToolDeleteWatchTask = "delete_watch_task"
)

func listWatchTasksExecutor(store watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, _ string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		tasks, err := store.ListTasks(ctx, session.UserID)
		if err != nil {
			return "", nil, err
		}
		if len(tasks) == 0 {
			return "目前没有追踪任务。", nil, nil
		}
		var text strings.Builder
		for _, task := range tasks {
			state := "启用"
			if !task.Enabled {
				state = "停用"
			}
			fmt.Fprintf(&text, "- id=%d %s %s:%s/%d [%s]\n", task.ID, task.ConditionType, task.TargetType, task.TargetText, task.TargetID, state)
		}
		return strings.TrimRight(text.String(), "\n"), nil, nil
	}
}

func createWatchTaskExecutor(store watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
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
		task, err := store.Create(ctx, watch.Task{
			UserID: session.UserID, ConditionType: args.ConditionType, TargetType: args.TargetType,
			TargetID: args.TargetID, TargetText: args.TargetText,
		})
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("已创建追踪 #%d（%s）。命中只会进入助手收件箱。", task.ID, task.ConditionType), nil, nil
	}
}

func updateWatchTaskExecutor(store watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID      int64 `json:"id"`
			Enabled bool  `json:"enabled"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "update_watch_task arguments are invalid")
		}
		if args.ID <= 0 {
			return "", nil, errx.NewWithCode(errx.ParamError)
		}
		if err := store.UpdateEnabled(ctx, session.UserID, args.ID, args.Enabled); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("已更新追踪 #%d。", args.ID), nil, nil
	}
}

func deleteWatchTaskExecutor(store watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID int64 `json:"id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "delete_watch_task arguments are invalid")
		}
		if args.ID <= 0 {
			return "", nil, errx.NewWithCode(errx.ParamError)
		}
		if err := store.Delete(ctx, session.UserID, args.ID); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("已删除追踪 #%d。", args.ID), nil, nil
	}
}
