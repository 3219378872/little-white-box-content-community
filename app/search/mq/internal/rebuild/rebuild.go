package rebuild

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/mq/internal/indexer"
	"esx/pkg/visibilityx"
)

// MaxPageSize 是单次从内容服务拉取的帖子上限，与 GetPostList 的分页上限一致。
const MaxPageSize int32 = 50

// PostSource 是重建所需的内容服务子集，便于测试替换。
type PostSource interface {
	GetPostList(context.Context, *contentservice.GetPostListReq, ...callopt.Option) (*contentservice.GetPostListResp, error)
}

// Target 是被重建的新索引；全部写入后 Refresh 一次，使结果在切换别名前可查。
type Target interface {
	Index(context.Context, indexer.IndexDoc) error
	Refresh(context.Context) error
}

// Run 按游标分页遍历内容服务的帖子，只把已发布帖子写入目标索引，返回写入数量；
// 任一页或任一文档失败即中止，由调用方丢弃半成品索引。
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
			// 草稿/取消发布的帖子不进入新索引（CORE-015）。
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
