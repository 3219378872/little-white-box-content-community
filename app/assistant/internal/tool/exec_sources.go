package tool

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"fmt"
	"strconv"
	"strings"
)

func presentSourcesExecutor(clients Clients) executorFunc {
	return func(ctx context.Context, session *Session, _ string, argsJSON string) (string, []store.SourceRef, error) {
		if clients.Store == nil || session == nil {
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
		found, err := clients.Store.GetSources(ctx, session.RunID, args.Handles)
		if err != nil {
			return "", nil, err
		}
		if len(found) == 0 {
			return "没有属于本 run 的有效 source handle。", nil, nil
		}
		postIDs := make([]int64, 0, len(found))
		for _, src := range found {
			if src.Kind != "post" {
				continue
			}
			id, parseErr := strconv.ParseInt(src.AuthorityID, 10, 64)
			if parseErr == nil && id > 0 {
				postIDs = append(postIDs, id)
			}
		}
		published := map[int64]*contentservice.PostInfo{}
		if len(postIDs) > 0 {
			published, err = publishedPosts(ctx, clients.Content, postIDs)
			if err != nil {
				return "", nil, err
			}
		}
		cards := make([]store.SourceRef, 0, len(found))
		var b strings.Builder
		b.WriteString("已展示来源卡：")
		for _, src := range found {
			card, ok := revalidateSource(ctx, clients, src, published)
			if !ok {
				continue
			}
			cards = append(cards, card)
			fmt.Fprintf(&b, " %s", src.Handle)
		}
		if len(cards) == 0 {
			return "来源已过期、变更或不可访问。", nil, nil
		}
		return b.String(), cards, nil
	}
}

func revalidateSource(ctx context.Context, clients Clients, src store.Source, published map[int64]*contentservice.PostInfo) (store.SourceRef, bool) {
	switch src.Kind {
	case "post":
		id, err := strconv.ParseInt(src.AuthorityID, 10, 64)
		if err != nil || id <= 0 {
			return store.SourceRef{}, false
		}
		info := published[id]
		if info == nil || info.Revision != src.Revision {
			return store.SourceRef{}, false
		}
		return store.SourceRef{
			Handle: src.Handle, Kind: src.Kind, AuthorityID: src.AuthorityID, Title: info.Title,
			Revision: info.Revision, PayloadJSON: sourcePayloadJSON(info.Title, truncateRunes(info.Content, maxEvidenceSnippetRunes), ""), Available: true,
		}, true
	case "web":
		if clients.Web == nil || strings.TrimSpace(src.AuthorityID) == "" {
			return store.SourceRef{}, false
		}
		results, err := clients.Web.Search(ctx, src.AuthorityID, 10)
		if err != nil {
			return store.SourceRef{}, false
		}
		for _, item := range results {
			if strings.TrimSpace(item.URL) == strings.TrimSpace(src.AuthorityID) {
				return store.SourceRef{
					Handle: src.Handle, Kind: src.Kind, AuthorityID: item.URL, Title: item.Title,
					PayloadJSON: sourcePayloadJSON(item.Title, truncateRunes(item.Content, maxEvidenceSnippetRunes), item.URL), Available: true,
				}, true
			}
		}
		return store.SourceRef{}, false
	default:
		return store.SourceRef{}, false
	}
}

func sourcePayloadJSON(title, snippet, url string) string {
	payload, _ := json.Marshal(map[string]string{"title": title, "snippet": snippet, "url": url})
	return string(payload)
}
