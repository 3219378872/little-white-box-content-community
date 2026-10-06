package visibility

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
)

// PostsByIDs is the Content authority used to verify published posts.
type PostsByIDs interface {
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...callopt.Option) (*contentservice.GetPostsByIdsResp, error)
}

// Fetch adapts a Content GetPostsByIds client to visibilityx.Fetcher.
// A nil client, RPC error, or nil response fails closed.
func Fetch(client PostsByIDs) visibilityx.Fetcher[*contentservice.PostInfo] {
	return fetch(client, false)
}

// fetch adapts the content client to visibilityx.Fetcher; a missing client or
// nil response is reported as service unavailable rather than "no posts".
func fetch(client PostsByIDs, skipTags bool) visibilityx.Fetcher[*contentservice.PostInfo] {
	return func(ctx context.Context, ids []int64) ([]*contentservice.PostInfo, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response, err := client.GetPostsByIds(ctx, &contentservice.GetPostsByIdsReq{PostIds: ids, SkipTags: skipTags})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		return response.Posts, nil
	}
}

// PublishedByIDs returns currently published posts from the Content authority.
func PublishedByIDs(ctx context.Context, client PostsByIDs, ids []int64) (map[int64]*contentservice.PostInfo, error) {
	return visibilityx.PublishedByIDs(ctx, Fetch(client), ids)
}

// PublishedByIDsWithoutTags is PublishedByIDs for callers that never read
// tags; Content skips the post_tag query and returns empty tags.
func PublishedByIDsWithoutTags(ctx context.Context, client PostsByIDs, ids []int64) (map[int64]*contentservice.PostInfo, error) {
	return visibilityx.PublishedByIDs(ctx, fetch(client, true), ids)
}
