package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
)

func researchDefinitions(clients Clients) []Definition {
	text := map[string]any{"type": "string"}
	option := objectSchema(map[string]any{"id": text, "label": text}, []string{"id", "label"})
	question := objectSchema(map[string]any{
		"id": text, "text": text, "selection": map[string]any{"type": "string", "enum": []string{"single", "multiple"}},
		"options": map[string]any{"type": "array", "minItems": 2, "maxItems": 8, "items": option},
	}, []string{"id", "text", "selection", "options"})
	citation := objectSchema(map[string]any{"handle": text, "evidenceIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": text}}, []string{"handle", "evidenceIds"})
	block := objectSchema(map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"fact", "experience", "inference", "context", "limitation"}},
		"text": text, "citations": map[string]any{"type": "array", "maxItems": 10, "items": citation},
	}, []string{"kind", "text", "citations"})
	return []Definition{
		{Name: AskQuestions, Description: "仅询问显著影响检索的未知条件，每批最多三问。用户可补充文字、未知、无偏好、跳过或先搜索。调用后持久等待真实答案，不作为授权或删除确认。", Parameters: objectSchema(map[string]any{"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": question}}, []string{"questions"}), executor: askQuestionsExecutor},
		{Name: ReadSource, Description: "分页读取本 run 来源的实际证据片段。cursor 为字符偏移，默认零；网页只能读取已取得的搜索摘录，不代表取得全文。", Parameters: objectSchema(map[string]any{"handle": text, "cursor": map[string]any{"type": "integer", "minimum": 0}}, []string{"handle"}), executor: readSourceExecutor(clients)},
		{Name: PublishAnswer, Description: "一次发布完整检索回答。fact/experience/inference 每段必须关联实际来源 handle 和 evidenceIds。context 仅用于用户自述/已知条件，limitation 仅说明检索缺口。不得把资料事实标成 context 逃避引用。成功后本 run 结束。", Parameters: objectSchema(map[string]any{"blocks": map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": block}}, []string{"blocks"}), executor: publishAnswerExecutor(clients)},
	}
}

func ForClient(registry *Registry, version int) *Registry {
	if registry == nil {
		return nil
	}
	var names []string
	for _, def := range registry.Definitions() {
		if version >= 2 && registry.Has(PublishAnswer) && def.Name == PresentSources {
			continue
		}
		meta, ok := registry.Metadata(def.Name)
		if ok && meta.MinClientProtocol <= version && def.MinClientProtocol <= version {
			names = append(names, def.Name)
		}
	}
	return registry.Restrict(names)
}

func askQuestionsExecutor(_ context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
	var args struct {
		Questions []store.Question `json:"questions"`
	}
	if err := strictUnmarshal(argsJSON, &args); err != nil {
		return "", nil, errx.New(errx.ParamError, "invalid questions")
	}
	if session == nil || session.Source != store.SourceUser || session.ClientProtocolVersion < 2 {
		return "", nil, errx.NewWithCode(errx.PermissionDenied)
	}
	if err := ValidateQuestions(args.Questions); err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%s", session.RunID, callID)))
	session.Question = &store.QuestionRequest{ID: "q_" + hex.EncodeToString(digest[:16]), RunID: session.RunID, UserID: session.UserID, CallID: callID, Status: "pending", Questions: args.Questions}
	return "等待用户回答。", nil, nil
}

func ValidateQuestions(questions []store.Question) error {
	if len(questions) < 1 || len(questions) > 3 {
		return errx.New(errx.ParamError, "questions must contain 1 to 3 items")
	}
	ids := map[string]bool{}
	for _, q := range questions {
		if q.ID == "" || len(q.ID) > 64 || ids[q.ID] || strings.TrimSpace(q.Text) == "" || utf8.RuneCountInString(q.Text) > 300 ||
			(q.Selection != "single" && q.Selection != "multiple") || len(q.Options) < 2 || len(q.Options) > 8 {
			return errx.New(errx.ParamError, "invalid question")
		}
		ids[q.ID] = true
		options := map[string]bool{}
		for _, o := range q.Options {
			if o.ID == "" || len(o.ID) > 64 || options[o.ID] || strings.TrimSpace(o.Label) == "" || utf8.RuneCountInString(o.Label) > 200 {
				return errx.New(errx.ParamError, "invalid question option")
			}
			options[o.ID] = true
		}
	}
	return nil
}

func evidenceFor(runID int64, handle, kind, text, commentID string) store.Evidence {
	digest := sha256.Sum256([]byte(handle + "\x00" + kind + "\x00" + commentID + "\x00" + text))
	return store.Evidence{ID: "ev_" + hex.EncodeToString(digest[:16]), RunID: runID, Handle: handle, Kind: kind, Text: text, CommentID: commentID, RetrievedAtMs: store.NowMs()}
}

func sourceExcerpt(src store.SourceRef) string {
	var payload struct {
		Snippet string `json:"snippet"`
	}
	if json.Unmarshal([]byte(src.PayloadJSON), &payload) == nil && payload.Snippet != "" {
		return payload.Snippet
	}
	return src.PayloadJSON
}

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
		found, err := clients.Store.GetSources(ctx, session.RunID, []string{args.Handle})
		if err != nil {
			return "", nil, err
		}
		if len(found) != 1 {
			return "", nil, errx.New(errx.NotFound, "source is not in this run")
		}
		src := found[0]
		var ref store.SourceRef
		if err := json.Unmarshal([]byte(src.PayloadJSON), &ref); err != nil {
			return "", nil, err
		}
		body := sourceExcerpt(ref)
		if src.Kind == "post" {
			info, err := currentPost(ctx, clients, session.UserID, src)
			if err != nil {
				return "", nil, err
			}
			body = info.Content
		}
		runes := []rune(body)
		if args.Cursor >= len(runes) {
			return `{"excerpts":[],"hasMore":false}`, nil, nil
		}
		end := min(args.Cursor+1200, len(runes))
		evidence := evidenceFor(session.RunID, src.Handle, src.Kind, string(runes[args.Cursor:end]), "")
		if session.Fence.Generation > 0 {
			err = clients.Store.RunStep(ctx, session.Fence, func(ctx context.Context, tx store.Store) error {
				run, err := tx.GetRun(ctx, session.RunID)
				if err != nil {
					return err
				}
				if run.CancelRequested {
					return errx.New(errx.ParamError, "run was cancelled")
				}
				return tx.PutEvidence(ctx, evidence)
			})
		} else {
			err = clients.Store.PutEvidence(ctx, evidence)
		}
		if err != nil {
			return "", nil, err
		}
		raw, _ := json.Marshal(map[string]any{"excerpts": []store.Evidence{evidence}, "nextCursor": end, "hasMore": end < len(runes)})
		return string(raw), nil, nil
	}
}

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

func publishAnswerExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		var args struct {
			Blocks []store.AnswerBlock `json:"blocks"`
		}
		if err := strictUnmarshal(argsJSON, &args); err != nil {
			return "", nil, errx.New(errx.ParamError, "invalid answer blocks")
		}
		answer, err := BuildAnswer(ctx, clients, session, args.Blocks)
		if err != nil {
			return "", nil, err
		}
		session.Answer = answer
		return "回答和来源已校验。", nil, nil
	}
}

func BuildAnswer(ctx context.Context, clients Clients, session *Session, blocks []store.AnswerBlock) (*store.AnswerPresentation, error) {
	if session == nil || session.UserID <= 0 || session.RunID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if len(blocks) == 0 || len(blocks) > 64 {
		return nil, errx.New(errx.ParamError, "answer requires 1 to 64 blocks")
	}
	answer := &store.AnswerPresentation{Version: 1, RunID: session.RunID, Blocks: blocks, Sources: []store.ResearchSource{}}
	byIdentity := map[string]int{}
	for i := range answer.Blocks {
		block := &answer.Blocks[i]
		block.Text = prompt.SanitizeOutput(block.Text)
		block.ID = fmt.Sprintf("b%d", i+1)
		if strings.TrimSpace(block.Text) == "" {
			return nil, errx.New(errx.ParamError, "empty answer block")
		}
		switch block.Kind {
		case "fact", "experience", "inference":
			if len(block.Citations) == 0 {
				return nil, errx.New(errx.ParamError, "retrieved statements require citations")
			}
		case "context", "limitation":
		default:
			return nil, errx.New(errx.ParamError, "invalid answer block kind")
		}
		for j := range block.Citations {
			citation := &block.Citations[j]
			found, err := clients.Store.GetSources(ctx, session.RunID, []string{citation.Handle})
			if err != nil {
				return nil, err
			}
			if len(found) != 1 || len(citation.EvidenceIDs) == 0 {
				return nil, errx.New(errx.ParamError, "unknown source or missing evidence")
			}
			src := found[0]
			var ref store.SourceRef
			if err := json.Unmarshal([]byte(src.PayloadJSON), &ref); err != nil {
				return nil, err
			}
			card := store.ResearchSource{Handle: src.Handle, Kind: src.Kind, AuthorityID: src.AuthorityID, Title: ref.Title, Revision: src.Revision, Available: true, Excerpts: []store.Evidence{}}
			var post *contentservice.PostInfo
			switch src.Kind {
			case "post":
				post, err = currentPost(ctx, clients, session.UserID, src)
				if err != nil {
					return nil, err
				}
				card.Title = post.Title
				card.URL = "/post/" + src.AuthorityID
				if len(post.Images) > 0 {
					card.ThumbnailURL = post.Images[0]
				}
			case "web":
				if !SafeSourceURL(src.AuthorityID) {
					return nil, errx.New(errx.ParamError, "unsafe source URL")
				}
				card.URL = src.AuthorityID
			default:
				return nil, errx.New(errx.ParamError, "unsupported evidence source")
			}
			evidence, err := clients.Store.ListEvidence(ctx, session.RunID, src.Handle)
			if err != nil {
				return nil, err
			}
			for _, id := range citation.EvidenceIDs {
				var selected *store.Evidence
				for k := range evidence {
					if evidence[k].ID == id {
						selected = &evidence[k]
						break
					}
				}
				if selected == nil || selected.Text == "" {
					return nil, errx.New(errx.ParamError, "evidence was not retrieved")
				}
				if selected.Kind == "post" && (post == nil || !strings.Contains(post.Content, selected.Text)) {
					return nil, errx.NewWithCode(errx.ContentVersionConflict)
				}
				if selected.Kind == "comment" {
					if err := validateCommentEvidence(ctx, clients, session.UserID, src, *selected); err != nil {
						return nil, err
					}
				}
				card.Excerpts = append(card.Excerpts, *selected)
			}
			identity := fmt.Sprintf("%s/%s/%d", card.Kind, card.AuthorityID, card.Revision)
			index, exists := byIdentity[identity]
			if !exists {
				if len(answer.Sources) >= 10 {
					return nil, errx.New(errx.ParamError, "at most 10 sources per answer")
				}
				index = len(answer.Sources)
				byIdentity[identity] = index
				answer.Sources = append(answer.Sources, card)
			} else {
				for _, ev := range card.Excerpts {
					duplicate := false
					for _, old := range answer.Sources[index].Excerpts {
						if old.ID == ev.ID {
							duplicate = true
							break
						}
					}
					if !duplicate {
						answer.Sources[index].Excerpts = append(answer.Sources[index].Excerpts, ev)
					}
				}
			}
			citation.Handle = answer.Sources[index].Handle
		}
	}
	return answer, nil
}

func AnswerText(answer *store.AnswerPresentation) string {
	var out strings.Builder
	for i, block := range answer.Blocks {
		if i > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(block.Text)
		seen := map[string]bool{}
		for _, citation := range block.Citations {
			if seen[citation.Handle] {
				continue
			}
			seen[citation.Handle] = true
			for j, src := range answer.Sources {
				if src.Handle == citation.Handle {
					fmt.Fprintf(&out, " [%d](%s)", j+1, src.URL)
					break
				}
			}
		}
	}
	return out.String()
}

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

func RevalidatePresentation(ctx context.Context, clients Clients, userID int64, answer *store.AnswerPresentation) {
	for i := range answer.Sources {
		card := &answer.Sources[i]
		var valid bool
		if card.Kind == "post" {
			source := store.Source{RunID: answer.RunID, Handle: card.Handle, Kind: card.Kind, AuthorityID: card.AuthorityID, Revision: card.Revision}
			post, err := currentPost(ctx, clients, userID, source)
			valid = err == nil
			if valid {
				for _, ev := range card.Excerpts {
					if ev.Kind == "comment" {
						valid = validateCommentEvidence(ctx, clients, userID, source, ev) == nil
					} else {
						valid = strings.Contains(post.Content, ev.Text)
					}
					if !valid {
						break
					}
				}
			}
		} else {
			valid = card.Kind == "web" && SafeSourceURL(card.URL)
		}
		if !valid {
			card.Available = false
			card.UnavailableReason = "source_unavailable"
			card.Title = ""
			card.ThumbnailURL = ""
			card.Author = ""
			card.Excerpts = []store.Evidence{}
		}
	}
}
