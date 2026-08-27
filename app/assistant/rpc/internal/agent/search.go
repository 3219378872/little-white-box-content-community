package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/rpc/internal/tool"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	ToolSearchUsers     = "search_users"
	ToolSearchTags      = "search_tags"
	ToolGetPost         = "get_post"
	ToolGetPostComments = "get_post_comments"

	rrfK                      = 60
	maxCommentPosts           = 3
	maxCommentsPerPost        = 3
	commentActiveStatus int32 = 1
	publishedPostStatus int32 = 1
)

func searchPostsExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []tool.Source, error) {
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
		page := args.Page
		if page <= 0 {
			page = 1
		}
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = defaultPageResult
		}
		sortBy := args.SortBy
		if sortBy <= 0 {
			sortBy = 1
			if session != nil && session.Plan.TimeRange == "recent" {
				sortBy = 2
			}
		}
		response, err := clients.Search.SearchPosts(ctx, &searchservice.SearchPostsReq{
			Keyword: keyword, Page: page, PageSize: pageSize, SortBy: sortBy, Tags: args.Tags,
		})
		if err != nil {
			logx.WithContext(ctx).Errorw("agent search_posts rpc failed", logx.Field("err", err.Error()))
			return "", nil, errx.FromRPCError(err)
		}
		esIDs := make([]int64, 0, len(response.GetPosts()))
		highlights := make(map[int64]string, len(response.GetPosts()))
		titles := make(map[int64]string, len(response.GetPosts()))
		for _, post := range response.GetPosts() {
			if post == nil || post.Id <= 0 {
				continue
			}
			esIDs = append(esIDs, post.Id)
			highlights[post.Id] = post.ContentHighlight
			titles[post.Id] = post.Title
		}

		seed := args.SeedPostID
		if seed <= 0 && session != nil {
			seed = session.ContextPostID
		}
		similarIDs := similarPostIDs(ctx, clients.Recommend, seed, pageSize, session)

		ordered := rrfMerge([][]int64{esIDs, similarIDs}, rrfK)
		if len(ordered) == 0 {
			ordered = esIDs
		}
		published, err := tool.PublishedPosts(ctx, clients.Content, ordered)
		if err != nil {
			logx.WithContext(ctx).Errorw("agent search_posts visibility check failed", logx.Field("err", err.Error()))
			return "", nil, err
		}

		includeComments := session != nil && session.Plan.Intent == IntentCommunityOpinion
		if args.IncludeComments != nil {
			includeComments = *args.IncludeComments
		}

		var text strings.Builder
		sources := make([]tool.Source, 0, len(ordered)+maxCommentPosts*maxCommentsPerPost)
		commented := 0
		for _, id := range ordered {
			info := published[id]
			if info == nil {
				continue
			}
			snippet := highlights[id]
			if strings.TrimSpace(snippet) == "" {
				snippet = info.Content
			}
			snippet = truncateRunes(snippet, maxEvidenceSnippetRunes)
			title := strings.TrimSpace(info.Title)
			if title == "" {
				title = titles[id]
			}
			fmt.Fprintf(&text, "- [post:%d] %s\n  摘要: %s\n", id, title, snippet)
			sources = append(sources, tool.Source{
				Type: "post", ID: strconv.FormatInt(id, 10), Title: title,
				Snippet: snippet, Revision: info.Revision,
			})
			if includeComments && commented < maxCommentPosts {
				commentText, commentSources := commentsForPost(ctx, clients.Content, info, keyword)
				text.WriteString(commentText)
				sources = append(sources, commentSources...)
				commented++
			}
		}
		if text.Len() == 0 {
			return "没有找到与关键词相关的已发布帖子。", nil, nil
		}
		return strings.TrimRight(text.String(), "\n"), sources, nil
	}
}

func similarPostIDs(ctx context.Context, recommend recommendservice.RecommendService, seed int64, limit int32, session *Session) []int64 {
	if recommend == nil || seed <= 0 {
		return nil
	}
	requestID := ""
	if session != nil {
		requestID = session.RequestID
	}
	response, err := recommend.GetSimilarPosts(ctx, &recommendservice.GetSimilarPostsReq{
		PostId:    seed,
		Limit:     limit,
		Scene:     "agent",
		RequestId: requestID,
	})
	if err != nil {
		logx.WithContext(ctx).Infow("agent similar posts degraded", logx.Field("err", err.Error()))
		return nil
	}
	ids := make([]int64, 0, len(response.GetPosts()))
	for _, post := range response.GetPosts() {
		if post == nil || post.PostId <= 0 || post.PostId == seed {
			continue
		}
		ids = append(ids, post.PostId)
	}
	return ids
}

// rrfMerge 用 Reciprocal Rank Fusion 合并多路有序列表，保留首次出现顺序下的去重。
func rrfMerge(lists [][]int64, k int) []int64 {
	if k <= 0 {
		k = rrfK
	}
	scores := make(map[int64]float64)
	order := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, list := range lists {
		for rank, id := range list {
			if id <= 0 {
				continue
			}
			scores[id] += 1 / float64(k+rank+1)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		return nil
	}
	sorted := append([]int64(nil), order...)
	for i := 0; i < len(sorted); i++ {
		best := i
		for j := i + 1; j < len(sorted); j++ {
			if scores[sorted[j]] > scores[sorted[best]] {
				best = j
			}
		}
		sorted[i], sorted[best] = sorted[best], sorted[i]
	}
	return sorted
}

func commentsForPost(ctx context.Context, content contentservice.ContentService, post *contentservice.PostInfo, keyword string) (string, []tool.Source) {
	if content == nil || post == nil {
		return "", nil
	}
	response, err := content.GetCommentList(ctx, &contentservice.GetCommentListReq{
		PostId: post.Id, Page: 1, PageSize: 5, SortBy: 2,
	})
	if err != nil {
		logx.WithContext(ctx).Infow("agent comment list degraded", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
		return "", nil
	}
	var text strings.Builder
	sources := make([]tool.Source, 0, maxCommentsPerPost)
	for _, comment := range response.GetComments() {
		if comment == nil || comment.Id <= 0 || comment.Status != commentActiveStatus {
			continue
		}
		snippet := truncateRunes(comment.Content, maxEvidenceSnippetRunes)
		if snippet == "" {
			continue
		}
		fmt.Fprintf(&text, "  - [comment:%d] %s\n", comment.Id, snippet)
		sources = append(sources, tool.Source{
			Type: "comment", ID: strconv.FormatInt(comment.Id, 10),
			Title: fmt.Sprintf("评论 · %s", post.Title), Snippet: snippet, Revision: post.Revision,
		})
		if len(sources) >= maxCommentsPerPost {
			break
		}
	}
	_ = keyword
	return text.String(), sources
}

func searchUsersExecutor(search searchservice.SearchService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []tool.Source, error) {
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
		keyword := strings.TrimSpace(args.Keyword)
		if keyword == "" {
			return "", nil, errx.New(errx.ParamError, "search_users keyword is required")
		}
		page := args.Page
		if page <= 0 {
			page = 1
		}
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = defaultPageResult
		}
		response, err := search.SearchUsers(ctx, &searchservice.SearchUsersReq{
			Keyword: keyword, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(response.GetUsers()) == 0 {
			return "没有找到匹配的用户。", nil, nil
		}
		var text strings.Builder
		for _, user := range response.GetUsers() {
			if user == nil || user.Id <= 0 {
				continue
			}
			name := strings.TrimSpace(user.Nickname)
			if name == "" {
				name = user.Username
			}
			fmt.Fprintf(&text, "- user:%d %s (@%s) 粉丝 %d\n", user.Id, name, user.Username, user.FollowerCount)
		}
		if text.Len() == 0 {
			return "没有找到匹配的用户。", nil, nil
		}
		text.WriteString("注意：用户搜索结果不是社区帖子证据，引用社区事实时仍须使用 [post:<id>] 或 [comment:<id>]。")
		return strings.TrimRight(text.String(), "\n"), nil, nil
	}
}

func searchTagsExecutor(search searchservice.SearchService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []tool.Source, error) {
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
		keyword := strings.TrimSpace(args.Keyword)
		if keyword == "" {
			return "", nil, errx.New(errx.ParamError, "search_tags keyword is required")
		}
		limit := args.Limit
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		response, err := search.SearchTags(ctx, &searchservice.SearchTagsReq{Keyword: keyword, Limit: limit})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		if len(response.GetTags()) == 0 {
			return "没有找到匹配的标签。", nil, nil
		}
		var text strings.Builder
		for _, tag := range response.GetTags() {
			if tag == nil || strings.TrimSpace(tag.Name) == "" {
				continue
			}
			fmt.Fprintf(&text, "- #%s （%d 篇帖子）\n", tag.Name, tag.PostCount)
		}
		if text.Len() == 0 {
			return "没有找到匹配的标签。", nil, nil
		}
		return strings.TrimRight(text.String(), "\n"), nil, nil
	}
}

func getPostExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID int64 `json:"post_id"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_post arguments are invalid")
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "get_post requires post_id")
		}
		published, err := tool.PublishedPosts(ctx, content, []int64{args.PostID})
		if err != nil {
			return "", nil, err
		}
		info := published[args.PostID]
		if info == nil || info.Status != publishedPostStatus {
			return "", nil, errx.NewWithCode(errx.ContentNotFound)
		}
		snippet := truncateRunes(info.Content, maxEvidenceSnippetRunes)
		text := fmt.Sprintf("[post:%d] %s\n%s", info.Id, info.Title, snippet)
		source := tool.Source{
			Type: "post", ID: strconv.FormatInt(info.Id, 10), Title: info.Title,
			Snippet: snippet, Revision: info.Revision,
		}
		return text, []tool.Source{source}, nil
	}
}

func getPostCommentsExecutor(content contentservice.ContentService) executorFunc {
	return func(ctx context.Context, _ *Session, _ string, argsJSON string) (string, []tool.Source, error) {
		if content == nil {
			return "", nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args struct {
			PostID   int64 `json:"post_id"`
			Page     int32 `json:"page"`
			PageSize int32 `json:"page_size"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "get_post_comments arguments are invalid")
		}
		if args.PostID <= 0 {
			return "", nil, errx.New(errx.ParamError, "get_post_comments requires post_id")
		}
		published, err := tool.PublishedPosts(ctx, content, []int64{args.PostID})
		if err != nil {
			return "", nil, err
		}
		post := published[args.PostID]
		if post == nil {
			return "", nil, errx.NewWithCode(errx.ContentNotFound)
		}
		page := args.Page
		if page <= 0 {
			page = 1
		}
		pageSize := args.PageSize
		if pageSize <= 0 || pageSize > 20 {
			pageSize = 5
		}
		response, err := content.GetCommentList(ctx, &contentservice.GetCommentListReq{
			PostId: args.PostID, Page: page, PageSize: pageSize, SortBy: 1,
		})
		if err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		var text strings.Builder
		sources := make([]tool.Source, 0)
		fmt.Fprintf(&text, "帖子 [post:%d] 《%s》的评论：\n", post.Id, post.Title)
		for _, comment := range response.GetComments() {
			if comment == nil || comment.Id <= 0 || comment.Status != commentActiveStatus {
				continue
			}
			snippet := truncateRunes(comment.Content, maxEvidenceSnippetRunes)
			if snippet == "" {
				continue
			}
			fmt.Fprintf(&text, "- [comment:%d] %s\n", comment.Id, snippet)
			sources = append(sources, tool.Source{
				Type: "comment", ID: strconv.FormatInt(comment.Id, 10),
				Title: fmt.Sprintf("评论 · %s", post.Title), Snippet: snippet, Revision: post.Revision,
			})
		}
		if len(sources) == 0 {
			return fmt.Sprintf("帖子 [post:%d] 目前没有可引用的有效评论。", post.Id), nil, nil
		}
		return strings.TrimRight(text.String(), "\n"), sources, nil
	}
}

func truncateRunes(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	return string([]rune(text)[:maxRunes]) + "…"
}
