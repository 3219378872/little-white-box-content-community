package tool

import (
	"context"
	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
	"fmt"
	"strconv"
	"strings"
)

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
		args.PostID = normalizeWatchPostID(session, args.PostID)
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
		args.PostID = normalizeWatchPostID(session, args.PostID)
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
		sources := []store.SourceRef{postSource(post.GetPost(), excerptRunes(post.GetPost().Content, 120))}
		for _, c := range resp.GetComments() {
			if c == nil || c.Status != commentActiveStatus {
				continue
			}
			fmt.Fprintf(&b, "- comment:%d %s\n", c.Id, truncateRunes(c.Content, 160))
			sources[0].Evidence = append(sources[0].Evidence, store.Evidence{Kind: "comment", Text: excerptRunes(c.Content, 160), CommentID: strconv.FormatInt(c.Id, 10)})
		}
		if b.Len() == 0 {
			return "没有可展示的评论。", sources, nil
		}
		return strings.TrimRight(b.String(), "\n"), sources, nil
	}
}

// LLMs commonly round large Snowflake IDs when copying them from a prompt.
// A Watch run may correct only to one of its own persisted hit IDs, keeping
// the normal ownership/visibility checks intact and preventing arbitrary
// nearby post lookups.

func normalizeWatchPostID(session *Session, requested int64) int64 {
	if session == nil || session.Source != "watch" || requested <= 0 || len(session.WatchPostIDs) == 0 {
		return requested
	}
	var candidate int64
	var distance int64 = 1025
	for _, id := range session.WatchPostIDs {
		if id <= 0 {
			continue
		}
		if id == requested {
			return id
		}
		delta := id - requested
		if delta < 0 {
			delta = -delta
		}
		if delta < distance {
			candidate, distance = id, delta
		} else if delta == distance {
			candidate = 0
		}
	}
	if candidate > 0 {
		return candidate
	}
	return requested
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
	return func(ctx context.Context, session *Session, callID string, argsJSON string) (string, []store.SourceRef, error) {
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
		req := &contentservice.UpdatePostReq{
			PostId: args.PostID, AuthorId: session.UserID, ExpectedRevision: expected,
			IdempotencyKey: deriveIdempotencyKey(session.RequestID, callID, "update"),
		}
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
	return func(ctx context.Context, session *Session, callID string, argsJSON string) (string, []store.SourceRef, error) {
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
		if _, err := content.DeletePost(ctx, &contentservice.DeletePostReq{
			PostId: args.PostID, AuthorId: session.UserID, ExpectedRevision: expected,
			IdempotencyKey: deriveIdempotencyKey(session.RequestID, callID, "delete"),
		}); err != nil {
			return "", nil, errx.FromRPCError(err)
		}
		return fmt.Sprintf("帖子 #%d 已删除。", args.PostID), nil, nil
	}
}

func postRevisionPreparer(content contentservice.ContentService) prepareFunc {
	return func(ctx context.Context, session *Session, argsJSON string) (string, error) {
		if content == nil || session == nil || session.UserID <= 0 {
			return "", errx.NewWithCode(errx.ServiceUnavailable)
		}
		var args map[string]any
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", errx.New(errx.ParamError, "post tool arguments are invalid")
		}
		postID, ok := numericInt64(args["post_id"])
		if !ok || postID <= 0 {
			return "", errx.New(errx.ParamError, "post tool requires post_id")
		}
		expected, _ := numericInt64(args["expected_revision"])
		current, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: postID, UserId: session.UserID})
		if err != nil {
			return "", errx.FromRPCError(err)
		}
		post := current.GetPost()
		if post == nil || post.GetAuthorId() != session.UserID || post.GetRevision() <= 0 {
			return "", errx.NewWithCode(errx.ContentForbidden)
		}
		if expected <= 0 {
			expected = post.GetRevision()
			args["expected_revision"] = expected
		} else if expected != post.GetRevision() {
			return "", errx.NewWithCode(errx.ContentVersionConflict)
		}
		raw, err := canonical.JSON(args)
		if err != nil {
			return "", errx.New(errx.ParamError, "post tool arguments are invalid")
		}
		return string(raw), nil
	}
}
