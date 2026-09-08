package feedback

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type hashReader interface {
	HgetallCtx(context.Context, string) (map[string]string, error)
}

type HiddenPostReader struct {
	redis   hashReader
	version string
}

func NewHiddenPostReader(client hashReader, version string) *HiddenPostReader {
	if version == "" {
		version = "v1"
	}
	return &HiddenPostReader{redis: client, version: version}
}

// HiddenPosts reads only the explicit negative-feedback boundary. A missing
// dependency must not be interpreted as an empty set of hidden posts.
func (r *HiddenPostReader) HiddenPosts(ctx context.Context, userID int64) (map[int64]struct{}, error) {
	result := make(map[int64]struct{})
	if userID <= 0 {
		return result, nil
	}
	if r == nil || r.redis == nil {
		return nil, fmt.Errorf("negative feedback unavailable")
	}
	values, err := r.redis.HgetallCtx(ctx, fmt.Sprintf("feature:%s:u:%d:negative", r.version, userID))
	if err != nil {
		return nil, err
	}
	for target := range values {
		if !strings.HasPrefix(target, "post:") {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(target, "post:"), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid negative feedback target")
		}
		result[id] = struct{}{}
	}
	return result, nil
}
