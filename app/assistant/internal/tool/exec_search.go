package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"
	"fmt"
	"strings"
)

func searchPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Search == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Keyword         string   `json:"keyword"`
			Page            int32    `json:"page"`
			PageSize        int32    `json:"page_size"`
			Tags            []string `json:"tags"`
			SortBy          int32    `json:"sort_by"`
			IncludeComments *bool    `json:"include_comments"`
			SeedPostID      int64    `json:"seed_post_id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "search_posts arguments are invalid")
		}
		keyword := strings.TrimSpace(args.Keyword)
		if keyword == "" {
			return "", nil, errx.New(errx.ParamError, "search_posts keyword is required")
		}
		page, pageSize := args.Page, args.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 || pageSize > 20 {
			pageSize = defaultPageResult
		}
		sortBy := args.SortBy
		if sortBy <= 0 {
			sortBy = 1
		}
		response, err := clients.Search.SearchPosts(ctx, &searchservice.SearchPostsReq{
			Keyword: keyword, Page: page, PageSize: pageSize, SortBy: sortBy, Tags: args.Tags,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(response.GetPosts()))
		for _, post := range response.GetPosts() {
			if post != nil && post.Id > 0 {
				ids = append(ids, post.Id)
			}
		}
		published, err := publishedPosts(ctx, clients.Content, ids)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0, len(ids))
		for _, id := range ids {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
		}
		text, sources := formatPosts("搜索结果：\n", infos)
		return text, sources, nil
	}
}

func searchUsersExecutor(search searchservice.SearchService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if search == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Keyword  string `json:"keyword"`
			Page     int32  `json:"page"`
			PageSize int32  `json:"page_size"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "search_users arguments are invalid")
		}
		if strings.TrimSpace(args.Keyword) == "" {
			return "", nil, errx.New(errx.ParamError, "search_users keyword is required")
		}
		page, pageSize := args.Page, args.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 || pageSize > 20 {
			pageSize = defaultPageResult
		}
		response, err := search.SearchUsers(ctx, &searchservice.SearchUsersReq{Keyword: args.Keyword, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(response.GetUsers()) == 0 {
			return "没有找到匹配的用户。", nil, nil
		}
		var b strings.Builder
		for _, user := range response.GetUsers() {
			if user == nil {
				continue
			}
			fmt.Fprintf(&b, "- user:%d %s\n", user.Id, user.Nickname)
		}
		b.WriteString("注意：用户搜索结果不是社区来源。")
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}

func searchTagsExecutor(search searchservice.SearchService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if search == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Keyword string `json:"keyword"`
			Limit   int32  `json:"limit"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "search_tags arguments are invalid")
		}
		if strings.TrimSpace(args.Keyword) == "" {
			return "", nil, errx.New(errx.ParamError, "search_tags keyword is required")
		}
		limit := args.Limit
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		response, err := search.SearchTags(ctx, &searchservice.SearchTagsReq{Keyword: args.Keyword, Limit: limit})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(response.GetTags()) == 0 {
			return "没有找到匹配的标签。", nil, nil
		}
		var b strings.Builder
		for _, tag := range response.GetTags() {
			if tag != nil {
				fmt.Fprintf(&b, "- #%s\n", tag.Name)
			}
		}
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}

func searchHistoryExecutor(h History) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if h == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args HistoryArgs
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "search_history arguments are invalid")
		}
		text, err := h.Search(ctx, session, args)
		return text, nil, err
	}
}
