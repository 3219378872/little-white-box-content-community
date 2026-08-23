package rebuild

import (
	"context"
	"fmt"
	"strconv"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/mq/internal/indexer"
	"esx/pkg/visibilityx"

	"google.golang.org/grpc"
)

const MaxPageSize int32 = 50

type PostSource interface {
	GetPostList(context.Context, *contentservice.GetPostListReq, ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

type Target interface {
	Index(context.Context, indexer.IndexDoc) error
	Refresh(context.Context) error
}

func Run(ctx context.Context, source PostSource, target Target, pageSize int32) (int64, error) {
	if source == nil || target == nil {
		return 0, fmt.Errorf("search rebuild requires source and target")
	}
	if pageSize <= 0 || pageSize > MaxPageSize {
		return 0, fmt.Errorf("search rebuild page size must be between 1 and %d", MaxPageSize)
	}

	var indexed int64
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return indexed, err
		}
		resp, err := source.GetPostList(ctx, &contentservice.GetPostListReq{
			PageSize: pageSize, SortBy: 1, Cursor: cursor,
		})
		if err != nil {
			return indexed, fmt.Errorf("load content page: %w", err)
		}
		if resp == nil {
			return indexed, fmt.Errorf("load content page: nil response")
		}

		for _, post := range resp.Posts {
			if post == nil || post.Id <= 0 || !visibilityx.IsPublished(post.Status) {
				continue
			}
			doc := indexer.IndexDoc{
				DocID:    strconv.FormatInt(post.Id, 10),
				Type:     "rebuild",
				Revision: post.Revision,
				Body: map[string]any{
					"post_id":       post.Id,
					"author_id":     post.AuthorId,
					"title":         post.Title,
					"body":          post.Content,
					"tags":          post.Tags,
					"like_count":    post.LikeCount,
					"comment_count": post.CommentCount,
					"created_at":    post.CreatedAt,
				},
			}
			if err := target.Index(ctx, doc); err != nil {
				return indexed, fmt.Errorf("index post %d: %w", post.Id, err)
			}
			indexed++
		}

		// 游标为空表示没有更多数据。
		if len(resp.Posts) == 0 || resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	if err := target.Refresh(ctx); err != nil {
		return indexed, fmt.Errorf("refresh rebuilt index: %w", err)
	}
	return indexed, nil
}
