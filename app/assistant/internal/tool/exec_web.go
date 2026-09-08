package tool

import (
	"context"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/websearch"
	"esx/pkg/errx"
	"fmt"
	"strings"
)

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
			if !SafeSourceURL(item.URL) {
				continue
			}
			src := store.SourceRef{Handle: randomHandle(), Kind: "web", AuthorityID: item.URL, Title: item.Title, PayloadJSON: excerptRunes(item.Content, maxEvidenceSnippetRunes)}
			fmt.Fprintf(&b, "- handle=%s %s %s\n", src.Handle, item.Title, truncateRunes(item.Content, maxEvidenceSnippetRunes))
			sources = append(sources, src)
		}
		if len(sources) == 0 {
			return "", nil, errx.New(errx.ServiceUnavailable, "web search returned no usable public sources")
		}
		return strings.TrimRight(b.String(), "\n"), sources, nil
	}
}
