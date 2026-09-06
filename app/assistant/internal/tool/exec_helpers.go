package tool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
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
	sum := sha256.Sum256([]byte(requestID + "\x00" + callID))
	return fmt.Sprintf("agent:%s:%x", action, sum[:])
}

func deriveMemoryRequestID(requestID, callID string) string {
	sum := sha256.Sum256([]byte(requestID + "\x00" + callID))
	return fmt.Sprintf("agent-memory:%x", sum[:24])
}
