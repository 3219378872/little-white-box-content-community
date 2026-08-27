package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/tool"
	"esx/pkg/errx"
)

const (
	ToolGetMemory    = "get_memory"
	ToolAddMemory    = "add_memory"
	ToolUpdateMemory = "update_memory"
	ToolDeleteMemory = "delete_memory"
)

func getMemoryExecutor(store memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Layer string `json:"layer"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_memory arguments are invalid")
		}
		items, err := store.List(ctx, session.UserID, strings.TrimSpace(args.Layer), time.Now())
		if err != nil {
			return "", nil, err
		}
		if len(items) == 0 {
			return "目前没有可展示的记忆。", nil, nil
		}
		var text strings.Builder
		for _, item := range items {
			mark := ""
			if !item.Confirmed() {
				mark = "（可能）"
			}
			if item.Suppressed {
				mark = "（已禁止记住）"
			}
			fmt.Fprintf(&text, "- id=%d [%s] %s=%s score=%.2f%s\n", item.ID, item.Layer, item.Dimension, item.Value, item.Score, mark)
		}
		return strings.TrimRight(text.String(), "\n"), nil, nil
	}
}

func addMemoryExecutor(store memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Layer     string  `json:"layer"`
			Dimension string  `json:"dimension"`
			Value     string  `json:"value"`
			Score     float64 `json:"score"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "add_memory arguments are invalid")
		}
		layer := strings.TrimSpace(args.Layer)
		if layer == "" {
			layer = memory.LayerProfile
		}
		value := strings.TrimSpace(args.Value)
		dimension := strings.TrimSpace(args.Dimension)
		if dimension == "" {
			dimension = "topic"
		}
		if value == "" {
			return "", nil, errx.New(errx.ParamError, "add_memory value is required")
		}
		err := store.Apply(ctx, session.UserID, memory.Candidate{
			Layer: layer, Dimension: dimension, Value: value, Score: args.Score,
			Source: memory.SourceExplicit, Confidence: 1,
		}, time.Now())
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("已记住 [%s] %s=%s。", layer, dimension, value), nil, nil
	}
}

func updateMemoryExecutor(store memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID         int64    `json:"id"`
			Value      *string  `json:"value"`
			Score      *float64 `json:"score"`
			Suppressed *bool    `json:"suppressed"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "update_memory arguments are invalid")
		}
		if args.ID <= 0 {
			return "", nil, errx.New(errx.ParamError, "update_memory requires id")
		}
		if err := store.Update(ctx, session.UserID, args.ID, memory.Patch{Value: args.Value, Score: args.Score, Suppressed: args.Suppressed}, time.Now()); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				return "", nil, errx.NewWithCode(errx.NotFound)
			}
			return "", nil, err
		}
		return fmt.Sprintf("已更新记忆 %d。", args.ID), nil, nil
	}
}

func deleteMemoryExecutor(store memory.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if store == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID int64 `json:"id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "delete_memory arguments are invalid")
		}
		if args.ID <= 0 {
			return "", nil, errx.New(errx.ParamError, "delete_memory requires id")
		}
		if err := store.Delete(ctx, session.UserID, args.ID); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				return "", nil, errx.NewWithCode(errx.NotFound)
			}
			return "", nil, err
		}
		return fmt.Sprintf("已删除记忆 %d。", args.ID), nil, nil
	}
}
