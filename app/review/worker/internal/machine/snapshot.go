package machine

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/review/internal/store"
	"esx/pkg/event"
)

// decodeSnapshot 从冻结的快照记录中取出送审快照。
func decodeSnapshot(content string) (event.ReviewSnapshot, error) {
	var frozen struct {
		Snapshot event.ReviewSnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(content), &frozen); err != nil {
		return event.ReviewSnapshot{}, fmt.Errorf("machine: decode snapshot: %w", err)
	}
	return frozen.Snapshot, nil
}

// LookupAdapter 把 store 适配为 cascade.Lookup。
type LookupAdapter struct{ Store *store.Store }

// LookupVerdict 查询同一内容指纹在该市场与政策版本下的已有结论；未命中时 found=false。
func (a LookupAdapter) LookupVerdict(ctx context.Context, hash, market, policyVersion string) (string, []string, string, bool, error) {
	v, err := a.Store.LookupVerdict(ctx, hash, market, policyVersion)
	if err != nil || v == nil {
		return "", nil, "", false, err
	}
	return v.Verdict, v.PolicyCodes(), v.Source, true, nil
}

// ApprovedMedia 返回哪些素材哈希已在过往结论中过审。
func (a LookupAdapter) ApprovedMedia(ctx context.Context, hashes []string) (map[string]bool, error) {
	return a.Store.ApprovedMedia(ctx, hashes)
}
