// Package vectorprojection resolves immutable Milvus projection revisions.
package vectorprojection

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// Bound a single query; reaching the cap fails closed instead of mistaking a
// truncated history for a complete version fence. Retention requires an offline
// rebuild, not deleting tombstones while old deliveries may still arrive.
const MaxHistoryRows = 16384

type Querier interface {
	Query(context.Context, string, []string, string, []string, ...client.SearchQueryOptionFunc) (client.ResultSet, error)
}
type Row struct {
	PostID, Revision int64
	Deleted          bool
	Vector           []float32
}

func Latest(ctx context.Context, q Querier, collection string, ids []int64, withVector bool) (map[int64]Row, error) {
	result := map[int64]Row{}
	if len(ids) == 0 {
		return result, nil
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("post ID must be positive")
		}
		parts[i] = strconv.FormatInt(id, 10)
	}
	fields := []string{"post_id", "revision", "deleted"}
	if withVector {
		fields = append(fields, "embedding")
	}
	rows, err := q.Query(ctx, collection, nil, "post_id in ["+strings.Join(parts, ",")+"]", fields, client.WithLimit(MaxHistoryRows), client.WithSearchQueryConsistencyLevel(entity.ClStrong))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return result, nil
	}
	postIDs, ok := rows.GetColumn("post_id").(*entity.ColumnInt64)
	if !ok {
		return nil, fmt.Errorf("projection post_id metadata missing")
	}
	revisions, ok := rows.GetColumn("revision").(*entity.ColumnInt64)
	if !ok {
		return nil, fmt.Errorf("projection revision metadata missing")
	}
	deleted, ok := rows.GetColumn("deleted").(*entity.ColumnBool)
	if !ok {
		return nil, fmt.Errorf("projection tombstone metadata missing")
	}
	n := postIDs.Len()
	if n >= MaxHistoryRows {
		return nil, fmt.Errorf("projection history query reached safety limit; rebuild required")
	}
	if revisions.Len() != n || deleted.Len() != n {
		return nil, fmt.Errorf("projection metadata length mismatch")
	}
	var vectors *entity.ColumnFloatVector
	if withVector {
		vectors, ok = rows.GetColumn("embedding").(*entity.ColumnFloatVector)
		if !ok || vectors.Len() != n {
			return nil, fmt.Errorf("projection embedding metadata missing")
		}
	}
	for i, id := range postIDs.Data() {
		rev := revisions.Data()[i]
		if rev <= 0 {
			return nil, fmt.Errorf("projection revision must be positive")
		}
		row := Row{PostID: id, Revision: rev, Deleted: deleted.Data()[i]}
		if withVector {
			row.Vector = vectors.Data()[i]
		}
		old, exists := result[id]
		if !exists || rev > old.Revision || (rev == old.Revision && row.Deleted) {
			result[id] = row
		}
	}
	return result, nil
}
