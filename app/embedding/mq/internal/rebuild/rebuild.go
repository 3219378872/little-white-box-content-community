package rebuild

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/visibilityx"

	"google.golang.org/grpc"
)

const MaxPageSize int32 = 50

var invalidCollectionRune = regexp.MustCompile(`[^A-Za-z0-9_]`)

type PostSource interface {
	GetPostList(context.Context, *contentservice.GetPostListReq, ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

type Target interface {
	UpsertBatch(context.Context, []vectorstore.Record) error
	Flush(context.Context) error
	Count(context.Context) (int64, error)
	PromoteAlias(context.Context, string) error
}

type Options struct {
	PageSize     int32
	BatchSize    int
	MaxAttempts  int
	RetryBackoff time.Duration
}

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

func retry(ctx context.Context, options Options, operation func() error) error {
	_, err := retryValue(ctx, options, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

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
