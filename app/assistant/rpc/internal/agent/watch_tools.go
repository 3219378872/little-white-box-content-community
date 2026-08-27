package agent

import (
	"context"
	"fmt"
	"strings"

	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
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

func createWatchTaskExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
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
		task := watch.Task{
			UserID: session.UserID, ConditionType: args.ConditionType, TargetType: args.TargetType,
			TargetID: args.TargetID, TargetText: args.TargetText,
		}
		if err := WatchLookups(clients).Validate(ctx, task); err != nil {
			return "", nil, err
		}
		created, err := clients.Watch.Create(ctx, task)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("已创建追踪 #%d（%s）。命中只会进入助手收件箱。", created.ID, created.ConditionType), nil, nil
	}
}

func WatchLookups(clients Clients) watch.Lookups {
	return watch.Lookups{
		Author: func(ctx context.Context, userID int64) error {
			return assertWatchAuthorExists(ctx, clients.User, userID)
		},
		Post: func(ctx context.Context, postID int64) error {
			return assertWatchPostPublished(ctx, clients.Content, postID)
		},
		Tag: func(ctx context.Context, name string) error {
			return assertWatchTagExists(ctx, clients.Search, clients.Content, name)
		},
	}
}

func assertWatchAuthorExists(ctx context.Context, user userservice.UserService, userID int64) error {
	if user == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	resp, err := user.GetUser(ctx, &userservice.GetUserReq{UserId: userID})
	if err != nil {
		return errx.FromRPCError(err)
	}
	if resp == nil || resp.User == nil || resp.User.Id <= 0 {
		return errx.New(errx.ParamError, "watch author does not exist")
	}
	return nil
}

func assertWatchPostPublished(ctx context.Context, content contentservice.ContentService, postID int64) error {
	if content == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	resp, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: postID})
	if err != nil {
		return errx.FromRPCError(err)
	}
	if resp == nil || resp.Post == nil || !visibilityx.IsPublished(resp.Post.Status) {
		return errx.New(errx.ParamError, "watch post is not published")
	}
	return nil
}

func assertWatchTagExists(ctx context.Context, search searchservice.SearchService, content contentservice.ContentService, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errx.New(errx.ParamError, "watch target_text is required")
	}
	if search != nil {
		resp, err := search.SearchTags(ctx, &searchservice.SearchTagsReq{Keyword: name, Limit: 20})
		if err != nil {
			return errx.FromRPCError(err)
		}
		for _, tag := range resp.GetTags() {
			if tag != nil && strings.EqualFold(strings.TrimSpace(tag.Name), name) {
				return nil
			}
		}
	}
	if content != nil {
		resp, err := content.GetTags(ctx, &contentservice.GetTagsReq{Limit: 100})
		if err != nil {
			return errx.FromRPCError(err)
		}
		for _, tag := range resp.GetTags() {
			if tag != nil && strings.EqualFold(strings.TrimSpace(tag.Name), name) {
				return nil
			}
		}
	}
	if search == nil && content == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	return errx.New(errx.ParamError, "watch tag does not exist")
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
