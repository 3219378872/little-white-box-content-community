package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
)

// publishAnswerExecutor 校验并保存最终回答；成功后 run 结束。
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

// BuildAnswer 校验模型提交的回答块并解析引用：每段清洗输出、编号，检索类陈述必须带引用；
// 引用的来源按 (类型, 权威 ID, 修订号) 合并为最多 10 张来源卡片，引用 handle 统一指向卡片。
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
		if err := validateAnswerBlock(*block); err != nil {
			return nil, err
		}
		for j := range block.Citations {
			citation := &block.Citations[j]
			card, err := resolveAnswerSource(ctx, clients, session, *citation)
			if err != nil {
				return nil, err
			}
			index, err := mergeSourceCard(answer, byIdentity, card)
			if err != nil {
				return nil, err
			}
			citation.Handle = answer.Sources[index].Handle
		}
	}
	return answer, nil
}

// validateAnswerBlock 要求段落非空；fact/experience/inference 必须有引用，
// context 与 limitation 可以不引用，其余类型非法。
func validateAnswerBlock(block store.AnswerBlock) error {
	if strings.TrimSpace(block.Text) == "" {
		return errx.New(errx.ParamError, "empty answer block")
	}
	switch block.Kind {
	case "fact", "experience", "inference":
		if len(block.Citations) == 0 {
			return errx.New(errx.ParamError, "retrieved statements require citations")
		}
	case "context", "limitation":
	default:
		return errx.New(errx.ParamError, "invalid answer block kind")
	}
	return nil
}

// mergeSourceCard 把同一来源同一修订的多次引用合并成一张卡片并去重证据片段，返回卡片下标。
func mergeSourceCard(answer *store.AnswerPresentation, byIdentity map[string]int, card store.ResearchSource) (int, error) {
	identity := fmt.Sprintf("%s/%s/%d", card.Kind, card.AuthorityID, card.Revision)
	index, exists := byIdentity[identity]
	if !exists {
		if len(answer.Sources) >= 10 {
			return 0, errx.New(errx.ParamError, "at most 10 sources per answer")
		}
		index = len(answer.Sources)
		byIdentity[identity] = index
		answer.Sources = append(answer.Sources, card)
		return index, nil
	}
	merged := &answer.Sources[index]
	for _, ev := range card.Excerpts {
		if !slices.ContainsFunc(merged.Excerpts, func(old store.Evidence) bool { return old.ID == ev.ID }) {
			merged.Excerpts = append(merged.Excerpts, ev)
		}
	}
	return index, nil
}

// AnswerText 把结构化回答渲染成纯文本，在每段末尾按来源序号追加链接，供不支持卡片的客户端使用。
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

// RevalidatePresentation 在展示已保存的回答前重新校验每张来源卡片；
// 失效的来源保留位置但清空内容并标记不可用，引用序号因此保持稳定。
func RevalidatePresentation(ctx context.Context, clients Clients, userID int64, answer *store.AnswerPresentation) {
	for i := range answer.Sources {
		card := &answer.Sources[i]
		if sourceCardValid(ctx, clients, userID, answer.RunID, *card) {
			continue
		}
		card.Available = false
		card.UnavailableReason = "source_unavailable"
		card.Title = ""
		card.ThumbnailURL = ""
		card.Author = ""
		card.Excerpts = []store.Evidence{}
	}
}

// sourceCardValid 判断来源是否仍可展示：帖子须仍为同一修订且每段证据仍在正文或评论中，
// 网页须仍是安全地址。
func sourceCardValid(ctx context.Context, clients Clients, userID, runID int64, card store.ResearchSource) bool {
	if card.Kind != "post" {
		return card.Kind == "web" && SafeSourceURL(card.URL)
	}
	source := store.Source{RunID: runID, Handle: card.Handle, Kind: card.Kind, AuthorityID: card.AuthorityID, Revision: card.Revision}
	post, err := currentPost(ctx, clients, userID, source)
	if err != nil {
		return false
	}
	for _, ev := range card.Excerpts {
		if ev.Kind == "comment" {
			if validateCommentEvidence(ctx, clients, userID, source, ev) != nil {
				return false
			}
		} else if !strings.Contains(post.Content, ev.Text) {
			return false
		}
	}
	return true
}

// resolveAnswerSource 把一条引用解析为来源卡片：来源必须属于本 run，引用的证据必须是本 run
// 实际读取过的片段，并且对帖子与评论再次确认原文仍然存在。
func resolveAnswerSource(ctx context.Context, clients Clients, session *Session, citation store.AnswerCitation) (store.ResearchSource, error) {
	found, err := clients.Store.GetSources(ctx, session.RunID, []string{citation.Handle})
	if err != nil {
		return store.ResearchSource{}, err
	}
	if len(found) != 1 || len(citation.EvidenceIDs) == 0 {
		return store.ResearchSource{}, errx.New(errx.ParamError, "unknown source or missing evidence")
	}
	src := found[0]
	card, post, err := newSourceCard(ctx, clients, session.UserID, src)
	if err != nil {
		return store.ResearchSource{}, err
	}
	evidence, err := clients.Store.ListEvidence(ctx, session.RunID, src.Handle)
	if err != nil {
		return store.ResearchSource{}, err
	}
	// 逐个核对被引用的证据片段。
	for _, id := range citation.EvidenceIDs {
		index := slices.IndexFunc(evidence, func(ev store.Evidence) bool { return ev.ID == id })
		if index < 0 || evidence[index].Text == "" {
			return store.ResearchSource{}, errx.New(errx.ParamError, "evidence was not retrieved")
		}
		selected := evidence[index]
		if selected.Kind == "post" && (post == nil || !strings.Contains(post.Content, selected.Text)) {
			return store.ResearchSource{}, errx.NewWithCode(errx.ContentVersionConflict)
		}
		if selected.Kind == "comment" {
			if err := validateCommentEvidence(ctx, clients, session.UserID, src, selected); err != nil {
				return store.ResearchSource{}, err
			}
		}
		card.Excerpts = append(card.Excerpts, selected)
	}
	return card, nil
}

// newSourceCard 生成不含证据的来源卡片。帖子用当前版本的标题与首图，并返回帖子供核对证据；
// 网页只接受安全地址。
func newSourceCard(ctx context.Context, clients Clients, userID int64, src store.Source) (store.ResearchSource, *contentservice.PostInfo, error) {
	var ref store.SourceRef
	if err := json.Unmarshal([]byte(src.PayloadJSON), &ref); err != nil {
		return store.ResearchSource{}, nil, err
	}
	card := store.ResearchSource{Handle: src.Handle, Kind: src.Kind, AuthorityID: src.AuthorityID, Title: ref.Title, Revision: src.Revision, Available: true, Excerpts: []store.Evidence{}}
	switch src.Kind {
	case "post":
		post, err := currentPost(ctx, clients, userID, src)
		if err != nil {
			return store.ResearchSource{}, nil, err
		}
		card.Title = post.Title
		card.URL = "/post/" + src.AuthorityID
		if len(post.Images) > 0 {
			card.ThumbnailURL = post.Images[0]
		}
		return card, post, nil
	case "web":
		if !SafeSourceURL(src.AuthorityID) {
			return store.ResearchSource{}, nil, errx.New(errx.ParamError, "unsafe source URL")
		}
		card.URL = src.AuthorityID
		return card, nil, nil
	default:
		return store.ResearchSource{}, nil, errx.New(errx.ParamError, "unsupported evidence source")
	}
}
