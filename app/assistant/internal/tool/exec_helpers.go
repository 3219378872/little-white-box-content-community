package tool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"
	"esx/pkg/pageutil"
	"esx/pkg/visibilityx"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// postSource 为帖子生成来源引用。
func postSource(info *contentservice.PostInfo, snippet string) store.SourceRef {
	return store.SourceRef{
		Handle: randomHandle(), Kind: "post", AuthorityID: strconv.FormatInt(info.Id, 10),
		Title: info.Title, Revision: info.Revision, PayloadJSON: snippet,
	}
}

// formatPosts 把帖子列表渲染为带 handle 的文本，并返回待登记的来源。
func formatPosts(prefix string, infos []*contentservice.PostInfo) (string, []store.SourceRef) {
	if len(infos) == 0 {
		return prefix + "没有可展示的已发布帖子。", nil
	}
	var b strings.Builder
	b.WriteString(prefix)
	sources := make([]store.SourceRef, 0, len(infos))
	for _, info := range infos {
		snippet := excerptRunes(info.Content, maxEvidenceSnippetRunes)
		src := postSource(info, snippet)
		fmt.Fprintf(&b, "- handle=%s 《%s》 %s\n", src.Handle, info.Title, snippet)
		sources = append(sources, src)
	}
	return strings.TrimRight(b.String(), "\n"), sources
}

// formatUserPosts 回源并渲染当前用户相关的帖子列表，只保留仍已发布的帖子。
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

// parsePage 解析分页参数并给出默认值，拒绝超过深分页上限的窗口。
func parsePage(argsJSON string) (int32, int32, error) {
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
	if _, err := pageutil.PageOffset(args.Page, args.PageSize); err != nil {
		return 0, 0, errx.New(errx.ParamError, "page window exceeds 10000")
	}
	return args.Page, args.PageSize, nil
}

// numericInt64 从 JSON 解码值中取整数，拒绝带小数的数字。
func numericInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case json.Number:
		v, err := typed.Int64()
		return v, err == nil
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	default:
		return 0, false
	}
}

// resolveAttachments 把模型给出的媒体 ID 限定在本次对话附件内，防止引用他人或任意媒体。
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

// assertMediaOwnership 确认媒体存在、归属当前用户且状态可用。
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

// validateTextLimits 校验标题、正文与标签的长度上限。
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

// sanitizeTags 去掉空白标签。
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

// deriveIdempotencyKey 由 run 请求 ID 与调用 ID 派生下游幂等键，同一调用重试得到相同的键。
func deriveIdempotencyKey(requestID, callID, action string) string {
	sum := sha256.Sum256([]byte(requestID + "\x00" + callID))
	return fmt.Sprintf("agent:%s:%x", action, sum[:])
}

// deriveMemoryRequestID 由 run 请求 ID 与调用 ID 派生记忆写入的请求 ID。
func deriveMemoryRequestID(requestID, callID string) string {
	sum := sha256.Sum256([]byte(requestID + "\x00" + callID))
	return fmt.Sprintf("agent-memory:%x", sum[:24])
}
