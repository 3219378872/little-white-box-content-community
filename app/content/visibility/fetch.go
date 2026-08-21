package visibility

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"google.golang.org/grpc"
)

// PostsByIDs is the Content authority used to verify published posts.
type PostsByIDs interface {
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error)
}

// Fetch adapts a Content GetPostsByIds client to visibilityx.Fetcher.
// A nil client, RPC error, or nil response fails closed.
func Fetch(client PostsByIDs) visibilityx.Fetcher[*contentservice.PostInfo] {
	return func(ctx context.Context, ids []int64) ([]*contentservice.PostInfo, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response, err := client.GetPostsByIds(ctx, &contentservice.GetPostsByIdsReq{PostIds: ids})
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
