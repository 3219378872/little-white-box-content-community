package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"

	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
)

// evidenceFor 生成证据片段；ID 由来源、类型、评论与正文派生，同一片段多次读取得到同一 ID。
func evidenceFor(runID int64, handle, kind, text, commentID string) store.Evidence {
	digest := sha256.Sum256([]byte(handle + "\x00" + kind + "\x00" + commentID + "\x00" + text))
	return store.Evidence{ID: "ev_" + hex.EncodeToString(digest[:16]), RunID: runID, Handle: handle, Kind: kind, Text: text, CommentID: commentID, RetrievedAtMs: store.NowMs()}
}

// sourceExcerpt 返回网页来源可读的内容：只有搜索摘录，没有则退回原始载荷。
func sourceExcerpt(src store.SourceRef) string {
	var payload struct {
		Snippet string `json:"snippet"`
	}
	if json.Unmarshal([]byte(src.PayloadJSON), &payload) == nil && payload.Snippet != "" {
		return payload.Snippet
	}
	return src.PayloadJSON
}

// readSourceExecutor 按字符游标分页读取本 run 已登记来源的正文，每页最多 1200 字符，
// 读到的片段作为证据落库，之后回答只能引用这些证据。
func readSourceExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if session == nil || session.UserID <= 0 || session.RunID <= 0 {
			return "", nil, errx.NewWithCode(errx.LoginRequired)
		}
		var args struct {
			Handle string `json:"handle"`
			Cursor int    `json:"cursor"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil || args.Handle == "" || args.Cursor < 0 {
			return "", nil, errx.NewWithCode(errx.ParamError)
		}
		// 只能读取本 run 登记过的来源。
		found, err := clients.Store.GetSources(ctx, session.RunID, []string{args.Handle})
		if err != nil {
			return "", nil, err
		}
		if len(found) != 1 {
			return "", nil, errx.New(errx.NotFound, "source is not in this run")
		}
		src := found[0]
		body, err := sourceBody(ctx, clients, session.UserID, src)
		if err != nil {
			return "", nil, err
		}
		// 按 rune 切页，避免把多字节字符截断。
		runes := []rune(body)
		if args.Cursor >= len(runes) {
			return `{"excerpts":[],"hasMore":false}`, nil, nil
		}
		end := min(args.Cursor+sourcePageRunes, len(runes))
		evidence := evidenceFor(session.RunID, src.Handle, src.Kind, string(runes[args.Cursor:end]), "")
		if err := saveEvidence(ctx, clients, session, evidence); err != nil {
			return "", nil, err
		}
		raw, _ := json.Marshal(map[string]any{"excerpts": []store.Evidence{evidence}, "nextCursor": end, "hasMore": end < len(runes)})
		return string(raw), nil, nil
	}
}

// sourcePageRunes 是 read_source 单页返回的最大字符数。
const sourcePageRunes = 1200

// sourceBody 返回来源可读的正文：帖子读取当前已发布版本的全文，网页只有搜索摘录。
func sourceBody(ctx context.Context, clients Clients, userID int64, src store.Source) (string, error) {
	var ref store.SourceRef
	if err := json.Unmarshal([]byte(src.PayloadJSON), &ref); err != nil {
		return "", err
	}
	if src.Kind != "post" {
		return sourceExcerpt(ref), nil
	}
	info, err := currentPost(ctx, clients, userID, src)
	if err != nil {
		return "", err
	}
	return info.Content, nil
}

// saveEvidence 落库证据片段。持有 run 栅栏时在同一步骤内复核 run 未被取消，
// 避免已取消的 run 继续写入证据。
func saveEvidence(ctx context.Context, clients Clients, session *Session, evidence store.Evidence) error {
	if session.Fence.Generation <= 0 {
		return clients.Store.PutEvidence(ctx, evidence)
	}
	return clients.Store.RunStep(ctx, session.Fence, func(ctx context.Context, tx store.Store) error {
		run, err := tx.GetRun(ctx, session.RunID)
		if err != nil {
			return err
		}
		if run.CancelRequested {
			return errx.New(errx.ParamError, "run was cancelled")
		}
		return tx.PutEvidence(ctx, evidence)
	})
}

// currentPost 以当前用户身份重新读取帖子，确认仍可见且修订号未变；引用必须指向检索时的同一版本。
func currentPost(ctx context.Context, clients Clients, userID int64, src store.Source) (*contentservice.PostInfo, error) {
	if clients.Content == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	id, err := strconv.ParseInt(src.AuthorityID, 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	resp, err := clients.Content.GetPost(ctx, &contentservice.GetPostReq{PostId: id, UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	info := resp.GetPost()
	if info == nil || !visibilityx.IsPublished(info.Status) {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if info.Revision != src.Revision {
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}
	return info, nil
}

// SafeSourceURL 只接受公网 http(s) 地址：拒绝内网、回环、localhost、带凭据与纯数字顶级域，
// 防止回答把用户引向内部服务。
func SafeSourceURL(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return false
	}
	host := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return false
	}
	if net.ParseIP(host) == nil {
		parts := strings.Split(host, ".")
		if len(parts) < 2 {
			return false
		}
		if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return false
		}
	}
	return true
}

// validateCommentEvidence 确认评论证据仍存在、属于该帖子、处于正常状态且仍包含引用原文。
func validateCommentEvidence(ctx context.Context, clients Clients, userID int64, source store.Source, ev store.Evidence) error {
	postID, err := strconv.ParseInt(source.AuthorityID, 10, 64)
	if err != nil {
		return errx.NewWithCode(errx.ParamError)
	}
	commentID, err := strconv.ParseInt(ev.CommentID, 10, 64)
	if err != nil || commentID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if clients.Content == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	response, err := clients.Content.GetCommentsByIds(ctx, &contentservice.GetCommentsByIdsReq{UserId: userID, PostId: postID, Ids: []int64{commentID}})
	if err != nil {
		return errx.FromRPCError(err)
	}
	for _, comment := range response.GetComments() {
		if comment.Id == commentID && comment.PostId == postID && comment.Status == commentActiveStatus && strings.Contains(comment.Content, ev.Text) {
			return nil
		}
	}
	return errx.New(errx.ContentVersionConflict, "comment evidence is no longer available")
}
