package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/websearch"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"github.com/zeromicro/go-zero/core/logx"
)

func postSource(info *contentservice.PostInfo, snippet string) store.SourceRef {
	return store.SourceRef{
		Handle: randomHandle(), Kind: "post", AuthorityID: strconv.FormatInt(info.Id, 10),
		Title: info.Title, Revision: info.Revision, PayloadJSON: snippet,
	}
}

func formatPosts(prefix string, infos []*contentservice.PostInfo) (string, []store.SourceRef) {
	if len(infos) == 0 {
		return prefix + "没有可展示的已发布帖子。", nil
	}
	var b strings.Builder
	b.WriteString(prefix)
	sources := make([]store.SourceRef, 0, len(infos))
	for _, info := range infos {
		snippet := truncateRunes(info.Content, maxEvidenceSnippetRunes)
		src := postSource(info, snippet)
		fmt.Fprintf(&b, "- handle=%s 《%s》 %s\n", src.Handle, info.Title, snippet)
		sources = append(sources, src)
	}
	return strings.TrimRight(b.String(), "\n"), sources
}

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

func getPostExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID int64 `json:"post_id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "get_post requires post_id")
		}
		userID := int64(0)
		if session != nil {
			userID = session.UserID
		}
		resp, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: args.PostID, UserId: userID})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if resp.GetPost() == nil || !visibilityx.IsPublished(resp.GetPost().Status) {
			return "帖子不可见或未发布。", nil, nil
		}
		text, sources := formatPosts("", []*contentservice.PostInfo{resp.GetPost()})
		return text, sources, nil
	}
}

func getPostCommentsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID   int64 `json:"post_id"`
			Page     int32 `json:"page"`
			PageSize int32 `json:"page_size"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "get_post_comments requires post_id")
		}
		userID := int64(0)
		if session != nil {
			userID = session.UserID
		}
		post, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: args.PostID, UserId: userID})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if post.GetPost() == nil || !visibilityx.IsPublished(post.GetPost().Status) {
			return "父帖不可见，无法读取评论。", nil, nil
		}
		page, pageSize := args.Page, args.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 || pageSize > 20 {
			pageSize = 5
		}
		resp, err := content.GetCommentList(ctx, &contentservice.GetCommentListReq{PostId: args.PostID, Page: page, PageSize: pageSize, SortBy: 2})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		var b strings.Builder
		sources := []store.SourceRef{postSource(post.GetPost(), truncateRunes(post.GetPost().Content, 120))}
		for _, c := range resp.GetComments() {
			if c == nil || c.Status != commentActiveStatus {
				continue
			}
			fmt.Fprintf(&b, "- comment:%d %s\n", c.Id, truncateRunes(c.Content, 160))
		}
		if b.Len() == 0 {
			return "没有可展示的评论。", sources, nil
		}
		return strings.TrimRight(b.String(), "\n"), sources, nil
	}
}

func recommendPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PageSize int32 `json:"page_size"`
		}
		_ = strictUnmarshal(argsJSON, &args)
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = 10
		}
		userID, requestID := int64(0), ""
		if session != nil {
			userID, requestID = session.UserID, session.RequestID
		}
		resp, err := clients.Recommend.GetRecommendPosts(ctx, &recommendservice.GetRecommendPostsReq{
			UserId: userID, Scene: "agent", RequestId: requestID, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(resp.GetPosts()))
		for _, p := range resp.GetPosts() {
			if p != nil && p.PostId > 0 {
				ids = append(ids, p.PostId)
			}
		}
		published, err := publishedPosts(ctx, clients.Content, ids)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0)
		for _, id := range ids {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
			if len(infos) >= 5 {
				break
			}
		}
		text, sources := formatPosts("推荐结果：\n", infos)
		return text, sources, nil
	}
}

func similarPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Recommend == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID int64 `json:"post_id"`
			Limit  int32 `json:"limit"`
		}
		_ = strictUnmarshal(argsJSON, &args)
		if args.PostID <= 0 && session != nil {
			args.PostID = session.ContextPostID
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "similar_posts requires post_id")
		}
		limit := args.Limit
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		requestID := ""
		if session != nil {
			requestID = session.RequestID
		}
		resp, err := clients.Recommend.GetSimilarPosts(ctx, &recommendservice.GetSimilarPostsReq{PostId: args.PostID, Limit: limit, Scene: "agent", RequestId: requestID})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0)
		for _, p := range resp.GetPosts() {
			if p != nil && p.PostId > 0 {
				ids = append(ids, p.PostId)
			}
		}
		published, err := publishedPosts(ctx, clients.Content, ids)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0)
		for _, id := range ids {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
		}
		text, sources := formatPosts("相似帖子：\n", infos)
		return text, sources, nil
	}
}

func comparePostsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		var args struct {
			PostIDs []int64 `json:"post_ids"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || len(args.PostIDs) < 2 || len(args.PostIDs) > 5 {
			return "", nil, errx.New(errx.ParamError, "compare_posts requires 2 to 5 post_ids")
		}
		published, err := publishedPosts(ctx, content, args.PostIDs)
		if err != nil {
			return "", nil, err
		}
		infos := make([]*contentservice.PostInfo, 0)
		for _, id := range args.PostIDs {
			if info := published[id]; info != nil {
				infos = append(infos, info)
			}
		}
		if len(infos) < 2 {
			return "可比较的已发布帖子不足两篇。", nil, nil
		}
		text, sources := formatPosts("比较：\n", infos)
		return text, sources, nil
	}
}

func getMyFavoritesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := clients.Interaction.GetFavoriteList(ctx, &interactionservice.GetFavoriteListReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserPosts(ctx, clients.Content, resp.GetPostIds(), "收藏")
	}
}

func getMyLikesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Interaction == nil || clients.Content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := clients.Interaction.GetLikeList(ctx, &interactionservice.GetLikeListReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return formatUserPosts(ctx, clients.Content, resp.GetPostIds(), "点赞")
	}
}

func getMyFollowingExecutor(user userservice.UserService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if user == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		page, pageSize := parsePage(argsJSON)
		resp, err := user.GetFollowing(ctx, &userservice.GetFollowingReq{UserId: userID, Page: page, PageSize: pageSize})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(resp.GetUsers()) == 0 {
			return "目前没有关注的人。", nil, nil
		}
		var b strings.Builder
		for _, info := range resp.GetUsers() {
			if info != nil {
				fmt.Fprintf(&b, "- user_id=%d %s\n", info.Id, info.Nickname)
			}
		}
		return strings.TrimRight(b.String(), "\n"), nil, nil
	}
}

func getMyPostsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		userID, err := sessionUserID(session)
		if err != nil {
			return "", nil, err
		}
		_, pageSize := parsePage(argsJSON)
		resp, err := content.GetUserPosts(ctx, &contentservice.GetUserPostsReq{UserId: userID, PageSize: pageSize, SortBy: 1})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		ids := make([]int64, 0, len(resp.GetPosts()))
		for _, p := range resp.GetPosts() {
			if p != nil {
				ids = append(ids, p.Id)
			}
		}
		return formatUserPosts(ctx, content, ids, "发布")
	}
}

func formatUserPosts(ctx context.Context, content contentservice.ContentService, ids []int64, kind string) (string, []store.SourceRef, error) {
	published, err := publishedPosts(ctx, content, ids)
	if err != nil {
		return "", nil, err
	}
	infos := make([]*contentservice.PostInfo, 0)
	for _, id := range ids {
		if info := published[id]; info != nil && visibilityx.IsPublished(info.Status) {
			infos = append(infos, info)
		}
	}
	text, sources := formatPosts(fmt.Sprintf("你的已发布%s帖子：\n", kind), infos)
	return text, sources, nil
}

func parsePage(argsJSON string) (int32, int32) {
	var args struct {
		Page     int32 `json:"page"`
		PageSize int32 `json:"page_size"`
	}
	_ = strictUnmarshal(argsJSON, &args)
	if args.Page <= 0 {
		args.Page = 1
	}
	if args.PageSize <= 0 || args.PageSize > 20 {
		args.PageSize = 10
	}
	return args.Page, args.PageSize
}

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
		entry, changeID, err := mem.Add(ctx, userID, args.Target, args.Content, session.RequestID+":"+callID, store.NowMs())
		if err != nil {
			return "", nil, err
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
		entry, changeID, err := mem.Replace(ctx, userID, args.ID, args.Content, args.Version, session.RequestID+":"+callID, store.NowMs())
		if err != nil {
			return "", nil, err
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
		changeID, err := mem.Remove(ctx, userID, args.ID, args.Version, session.RequestID+":"+callID, store.NowMs())
		if err != nil {
			return "", nil, err
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
		_, ids, err := mem.Batch(ctx, userID, session.RequestID+":"+callID, args.Ops, store.NowMs())
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("批量记忆完成 changes=%v", ids), nil, nil
	}
}

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
			ID      int64 `json:"id"`
			Enabled bool  `json:"enabled"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "update_watch_task arguments are invalid")
		}
		if err := w.UpdateEnabled(ctx, session.UserID, args.ID, args.Enabled); err != nil {
			return "", nil, err
		}
		return "追踪已更新。", nil, nil
	}
}

func deleteWatchTaskExecutor(w watch.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if w == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			ID int64 `json:"id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "delete_watch_task arguments are invalid")
		}
		if err := w.Delete(ctx, session.UserID, args.ID); err != nil {
			return "", nil, err
		}
		return "追踪已删除。", nil, nil
	}
}

func webSearchExecutor(searcher websearch.Searcher) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if searcher == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Query string `json:"query"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || strings.TrimSpace(args.Query) == "" {
			return "", nil, errx.New(errx.ParamError, "web_search query is required")
		}
		results, err := searcher.Search(ctx, args.Query, 0)
		if err != nil {
			return "", nil, errx.New(errx.ServiceUnavailable, "web search provider unavailable")
		}
		if len(results) == 0 {
			return "网络搜索没有返回结果。", nil, nil
		}
		var b strings.Builder
		sources := make([]store.SourceRef, 0, len(results))
		for _, item := range results {
			src := store.SourceRef{Handle: randomHandle(), Kind: "web", AuthorityID: item.URL, Title: item.Title, PayloadJSON: truncateRunes(item.Content, maxEvidenceSnippetRunes)}
			fmt.Fprintf(&b, "- handle=%s %s\n", src.Handle, item.Title)
			sources = append(sources, src)
		}
		return strings.TrimRight(b.String(), "\n"), sources, nil
	}
}

func createPostExecutor(content contentservice.ContentService, media mediaservice.MediaService) executorFunc {
	return func(ctx context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Title         string   `json:"title"`
			Content       string   `json:"content"`
			Tags          []string `json:"tags"`
			ImageMediaIDs []int64  `json:"image_media_ids"`
			Status        *int32   `json:"status"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "create_post arguments are invalid")
		}
		title, body := strings.TrimSpace(args.Title), strings.TrimSpace(args.Content)
		if title == "" || body == "" {
			return "", nil, errx.New(errx.ParamError, "create_post requires title and content")
		}
		if err := validateTextLimits(title, body, args.Tags); err != nil {
			return "", nil, err
		}
		status := int32(1)
		if args.Status != nil {
			status = *args.Status
		}
		mediaIDs, urls, err := resolveAttachments(session, args.ImageMediaIDs)
		if err != nil {
			return "", nil, err
		}
		if len(mediaIDs) > 0 && media != nil {
			if err := assertMediaOwnership(ctx, media, session.UserID, mediaIDs); err != nil {
				return "", nil, err
			}
		}
		resp, err := content.CreatePost(ctx, &contentservice.CreatePostReq{
			AuthorId: session.UserID, Title: title, Content: body, Images: urls, Tags: sanitizeTags(args.Tags),
			Status: status, IdempotencyKey: deriveIdempotencyKey(session.RequestID, callID, "create"), MediaIds: mediaIDs,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return fmt.Sprintf("帖子创建成功: post_id=%d revision=%d。", resp.PostId, resp.Revision), nil, nil
	}
}

func updatePostExecutor(content contentservice.ContentService, media mediaservice.MediaService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID           int64    `json:"post_id"`
			Title            string   `json:"title"`
			Content          string   `json:"content"`
			Tags             []string `json:"tags"`
			ImageMediaIDs    []int64  `json:"image_media_ids"`
			Status           *int32   `json:"status"`
			ExpectedRevision int64    `json:"expected_revision"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "update_post requires post_id")
		}
		expected := args.ExpectedRevision
		if expected <= 0 {
			current, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: args.PostID, UserId: session.UserID})
			if err != nil {
				return "", nil, errx.FromRPCError(err)
			}
			expected = current.GetPost().GetRevision()
		}
		req := &contentservice.UpdatePostReq{PostId: args.PostID, AuthorId: session.UserID, ExpectedRevision: expected}
		if title := strings.TrimSpace(args.Title); title != "" {
			req.Title = title
		}
		if body := strings.TrimSpace(args.Content); body != "" {
			req.Content = body
		}
		if args.Tags != nil {
			req.Tags = sanitizeTags(args.Tags)
		}
		if args.Status != nil {
			req.Status = args.Status
		}
		if args.ImageMediaIDs != nil {
			ids, urls, err := resolveAttachments(session, args.ImageMediaIDs)
			if err != nil {
				return "", nil, err
			}
			if len(ids) > 0 && media != nil {
				if err := assertMediaOwnership(ctx, media, session.UserID, ids); err != nil {
					return "", nil, err
				}
			}
			req.Images, req.MediaIds = urls, ids
		}
		resp, err := content.UpdatePost(ctx, req)
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return fmt.Sprintf("帖子更新成功: post_id=%d revision=%d。", args.PostID, resp.Revision), nil, nil
	}
}

func deletePostExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID           int64 `json:"post_id"`
			ExpectedRevision int64 `json:"expected_revision"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "delete_post requires post_id")
		}
		expected := args.ExpectedRevision
		if expected <= 0 {
			current, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: args.PostID, UserId: session.UserID})
			if err != nil {
				return "", nil, errx.FromRPCError(err)
			}
			expected = current.GetPost().GetRevision()
		}
		if _, err := content.DeletePost(ctx, &contentservice.DeletePostReq{PostId: args.PostID, AuthorId: session.UserID, ExpectedRevision: expected}); err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return fmt.Sprintf("帖子 #%d 已删除。", args.PostID), nil, nil
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

func presentSourcesExecutor(st store.Store) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if st == nil || session == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			Handles []string `json:"handles"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "present_sources arguments are invalid")
		}
		if len(args.Handles) == 0 {
			return "没有可展示的来源。", nil, nil
		}
		if len(args.Handles) > 10 {
			args.Handles = args.Handles[:10]
		}
		found, err := st.GetSources(ctx, session.RunID, args.Handles)
		if err != nil {
			return "", nil, err
		}
		if len(found) == 0 {
			return "没有属于本 run 的有效 source handle。", nil, nil
		}
		cards := make([]store.SourceRef, 0, len(found))
		var b strings.Builder
		b.WriteString("已展示来源卡：")
		for _, src := range found {
			cards = append(cards, store.SourceRef{
				Handle: src.Handle, Kind: src.Kind, AuthorityID: src.AuthorityID, Title: titleFromPayload(src),
				Revision: src.Revision, PayloadJSON: src.PayloadJSON, Available: true,
			})
			fmt.Fprintf(&b, " %s", src.Handle)
		}
		return b.String(), cards, nil
	}
}

func titleFromPayload(src store.Source) string {
	if src.PayloadJSON == "" {
		return src.Kind
	}
	return src.Kind
}

func resolveAttachments(session *Session, requested []int64) ([]int64, []string, error) {
	if len(requested) == 0 {
		return nil, nil, nil
	}
	if len(requested) > maxPostImages {
		return nil, nil, errx.New(errx.ParamError, "too many images for a single post")
	}
	byID := map[int64]string{}
	if session != nil {
		for _, a := range session.Attachments {
			byID[a.MediaID] = a.URL
		}
	}
	ids := make([]int64, 0, len(requested))
	urls := make([]string, 0, len(requested))
	for _, id := range requested {
		url, ok := byID[id]
		if !ok {
			return nil, nil, errx.New(errx.ParamError, "image mediaId is not part of this conversation's attachments")
		}
		ids = append(ids, id)
		urls = append(urls, url)
	}
	return ids, urls, nil
}

func assertMediaOwnership(ctx context.Context, media mediaservice.MediaService, userID int64, mediaIDs []int64) error {
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: mediaIDs})
	if err != nil {
		return errx.FromRPCError(err)
	}
	found := map[int64]*mediaservice.MediaInfo{}
	for _, info := range response.GetMedias() {
		found[info.GetId()] = info
	}
	for _, id := range mediaIDs {
		info := found[id]
		switch {
		case info == nil:
			return errx.New(errx.MediaNotFound, "attached media does not exist")
		case info.GetUserId() != userID:
			return errx.New(errx.PermissionDenied, "attached media belongs to another user")
		case info.GetStatus() != 1:
			return errx.New(errx.ParamError, "attached media is not available")
		}
	}
	return nil
}

func validateTextLimits(title, content string, tags []string) error {
	if title != "" && utf8.RuneCountInString(title) > maxTitleRunes {
		return errx.New(errx.ParamError, "title exceeds 120 characters")
	}
	if content != "" && utf8.RuneCountInString(content) > maxContentRunes {
		return errx.New(errx.ParamError, "content exceeds 20000 characters")
	}
	if len(tags) > maxTags {
		return errx.New(errx.ParamError, "too many tags")
	}
	for _, tag := range tags {
		if utf8.RuneCountInString(tag) > maxTagRunes {
			return errx.New(errx.ParamError, "tag exceeds 32 characters")
		}
	}
	return nil
}

func sanitizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func deriveIdempotencyKey(requestID, callID, action string) string {
	key := fmt.Sprintf("agent:%s:%s:%s", action, requestID, callID)
	if len(key) > 128 {
		key = key[:128]
	}
	return key
}

var _ = logx.WithContext
