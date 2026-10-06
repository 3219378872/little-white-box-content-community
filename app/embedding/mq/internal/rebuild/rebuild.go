package rebuild

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/visibilityx"
)

const MaxPageSize int32 = 50

var invalidCollectionRune = regexp.MustCompile(`[^A-Za-z0-9_]`)

// PostSource 是重建所需的内容服务子集，便于测试替换。
type PostSource interface {
	GetPostList(context.Context, *contentservice.GetPostListReq, ...callopt.Option) (*contentservice.GetPostListResp, error)
}

// Target 是被重建的新集合：批量写入、落盘、计数与别名切换。
type Target interface {
	UpsertBatch(context.Context, []vectorstore.Record) error
	Flush(context.Context) error
	Count(context.Context) (int64, error)
	PromoteAlias(context.Context, string) error
}

// Options 是重建的分页、批量与重试参数。
type Options struct {
	PageSize     int32
	BatchSize    int
	MaxAttempts  int
	RetryBackoff time.Duration
}

// VersionedCollectionName 生成带模型版本与时间戳的新集合名，非法字符替换为下划线并截断到 Milvus 长度上限。
func VersionedCollectionName(prefix, modelVersion string, now time.Time) (string, error) {
	prefix = strings.Trim(invalidCollectionRune.ReplaceAllString(prefix, "_"), "_")
	modelVersion = strings.Trim(invalidCollectionRune.ReplaceAllString(modelVersion, "_"), "_")
	if prefix == "" || modelVersion == "" {
		return "", fmt.Errorf("collection prefix and model version must contain letters or digits")
	}
	timestamp := now.UTC().Format("20060102_150405_000000000")
	maxBaseLength := 255 - len(timestamp) - 1
	base := prefix + "_" + modelVersion
	if len(base) > maxBaseLength {
		base = strings.TrimRight(base[:maxBaseLength], "_")
	}
	name := base + "_" + timestamp
	if name[0] >= '0' && name[0] <= '9' {
		name = "v" + name[1:]
	}
	return name, nil
}

// RunAndPromote 分页读取已发布帖子、批量生成向量写入新集合，落盘后核对行数，一致才切换别名；
// 各步骤失败按退避重试，返回写入数量。
func RunAndPromote(
	ctx context.Context,
	source PostSource,
	emb embedder.BatchEmbedder,
	target Target,
	alias string,
	options Options,
) (int64, error) {
	if source == nil || emb == nil || target == nil {
		return 0, fmt.Errorf("embedding rebuild requires content source, embedder, and target")
	}
	if strings.TrimSpace(alias) == "" {
		return 0, fmt.Errorf("embedding rebuild alias is required")
	}
	if err := validateOptions(options); err != nil {
		return 0, err
	}

	var indexed int64
	cursor := ""
	for {
		resp, err := retryValue(ctx, options, func() (*contentservice.GetPostListResp, error) {
			return source.GetPostList(ctx, &contentservice.GetPostListReq{
				PageSize: options.PageSize, SortBy: 1, Cursor: cursor,
			})
		})
		if err != nil {
			return indexed, fmt.Errorf("load content page: %w", err)
		}
		if resp == nil {
			return indexed, fmt.Errorf("load content page: nil response")
		}
		posts := publishedPosts(resp.Posts)
		for start := 0; start < len(posts); start += options.BatchSize {
			end := min(start+options.BatchSize, len(posts))
			batch := posts[start:end]
			texts := make([]string, len(batch))
			for i, post := range batch {
				if post.Revision <= 0 {
					return indexed, fmt.Errorf("published post %d has no authoritative revision", post.Id)
				}
				texts[i] = post.Title + "\n" + post.Content
			}
			results, err := retryValue(ctx, options, func() ([]embedder.Embedding, error) {
				return emb.EmbedBatch(ctx, texts)
			})
			if err != nil {
				return indexed, fmt.Errorf("embed content batch %d: %w", start/options.BatchSize+1, err)
			}
			if len(results) != len(batch) {
				return indexed, fmt.Errorf("embedding batch result count mismatch: got %d, want %d", len(results), len(batch))
			}
			records := make([]vectorstore.Record, len(batch))
			for i, post := range batch {
				records[i] = vectorstore.Record{
					PostID:       post.Id,
					Revision:     post.Revision,
					Vector:       results[i].Vector,
					ModelVersion: results[i].ModelVersion,
					Dimension:    results[i].Dimension,
				}
			}
			if err := retry(ctx, options, func() error { return target.UpsertBatch(ctx, records) }); err != nil {
				return indexed, fmt.Errorf("upsert content batch %d: %w", start/options.BatchSize+1, err)
			}
			indexed += int64(len(records))
		}

		// 游标为空表示没有更多数据。
		if len(resp.Posts) == 0 || resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	if err := retry(ctx, options, func() error { return target.Flush(ctx) }); err != nil {
		return indexed, fmt.Errorf("flush rebuilt embeddings: %w", err)
	}
	if err := retry(ctx, options, func() error {
		count, err := target.Count(ctx)
		if err != nil {
			return err
		}
		if count != indexed {
			return fmt.Errorf("rebuilt row count mismatch: got %d, want %d", count, indexed)
		}
		return nil
	}); err != nil {
		return indexed, fmt.Errorf("verify rebuilt embeddings: %w", err)
	}
	if err := retry(ctx, options, func() error { return target.PromoteAlias(ctx, alias) }); err != nil {
		return indexed, fmt.Errorf("promote rebuilt embeddings: %w", err)
	}
	return indexed, nil
}

// validateOptions 校验重建参数均为正且分页不超过上限。
func validateOptions(options Options) error {
	if options.PageSize <= 0 || options.PageSize > MaxPageSize {
		return fmt.Errorf("embedding rebuild page size must be between 1 and %d", MaxPageSize)
	}
	if options.BatchSize <= 0 {
		return fmt.Errorf("embedding rebuild batch size must be positive")
	}
	if options.MaxAttempts <= 0 {
		return fmt.Errorf("embedding rebuild max attempts must be positive")
	}
	if options.RetryBackoff <= 0 {
		return fmt.Errorf("embedding rebuild retry backoff must be positive")
	}
	return nil
}

// publishedPosts 只保留已发布的帖子（CORE-015）。
func publishedPosts(posts []*contentservice.PostInfo) []*contentservice.PostInfo {
	result := make([]*contentservice.PostInfo, 0, len(posts))
	for _, post := range posts {
		if post == nil || post.Id <= 0 || !visibilityx.IsPublished(post.Status) {
			continue
		}
		result = append(result, post)
	}
	return result
}

// retry 按重试策略执行无返回值的操作。
func retry(ctx context.Context, options Options, operation func() error) error {
	_, err := retryValue(ctx, options, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// retryValue 最多重试 MaxAttempts 次，第 n 次失败后等待 n 倍退避；上下文取消时立即返回。
func retryValue[T any](ctx context.Context, options Options, operation func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 1; attempt <= options.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		result, err := operation()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == options.MaxAttempts {
			break
		}
		timer := time.NewTimer(options.RetryBackoff * time.Duration(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, fmt.Errorf("failed after %d attempts: %w", options.MaxAttempts, lastErr)
}
