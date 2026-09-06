package tool

import (
	"context"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
	"fmt"
	"strings"
)

func readMemoryExecutor(mem memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if mem == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args struct {
			Target string `json:"target"`
		}
		_ = strictUnmarshal(argsJSON, &args)
		items, caps, err := mem.List(ctx, userID, args.Target)
		if err != nil {
			return "", nil, err
		}
		if len(items) == 0 {
			return "当前没有记忆条目。", nil, nil
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "- %s#%d v%d %s\n", item.Target, item.ID, item.Version, item.Content)
		}
		for _, cap := range caps {
			fmt.Fprintf(&b, "容量 %s %d/%d\n", cap.Target, cap.Used, cap.Limit)
		}
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}

func addMemoryExecutor(mem memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
		if mem == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args struct {
			Target  string `json:"target"`
			Content string `json:"content"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "add_memory arguments are invalid")
		}
		entry, changeID, err := mem.Add(ctx, userID, args.Target, args.Content, deriveMemoryRequestID(session.RequestID, callID), store.NowMs())
		if err != nil {
			return "", nil, err
		}
		if changeID > 0 {
			session.ChangeIDs = append(session.ChangeIDs, changeID)
		}
		return fmt.Sprintf("已写入 %s#%d change=%d", entry.Target, entry.ID, changeID), nil, nil
	}
}

func replaceMemoryExecutor(mem memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
		if mem == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args struct {
			ID      int64  `json:"id"`
			Content string `json:"content"`
			Version int32  `json:"version"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "replace_memory arguments are invalid")
		}
		entry, changeID, err := mem.Replace(ctx, userID, args.ID, args.Content, args.Version, deriveMemoryRequestID(session.RequestID, callID), store.NowMs())
		if err != nil {
			return "", nil, err
		}
		if changeID > 0 {
			session.ChangeIDs = append(session.ChangeIDs, changeID)
		}
		return fmt.Sprintf("已替换 %s#%d v%d change=%d", entry.Target, entry.ID, entry.Version, changeID), nil, nil
	}
}

func removeMemoryExecutor(mem memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
		if mem == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args struct {
			ID      int64 `json:"id"`
			Version int32 `json:"version"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "remove_memory arguments are invalid")
		}
		changeID, err := mem.Remove(ctx, userID, args.ID, args.Version, deriveMemoryRequestID(session.RequestID, callID), store.NowMs())
		if err != nil {
			return "", nil, err
		}
		if changeID > 0 {
			session.ChangeIDs = append(session.ChangeIDs, changeID)
		}
		return fmt.Sprintf("已删除记忆 change=%d", changeID), nil, nil
	}
}

func batchMemoryExecutor(mem memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
		if mem == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		var args struct {
			Ops []memory.Op `json:"ops"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "batch_memory arguments are invalid")
		}
		_, ids, err := mem.Batch(ctx, userID, deriveMemoryRequestID(session.RequestID, callID), args.Ops, store.NowMs())
		if err != nil {
			return "", nil, err
		}
		session.ChangeIDs = append(session.ChangeIDs, ids...)
		return fmt.Sprintf("批量记忆完成 changes=%v", ids), nil, nil
	}
}
