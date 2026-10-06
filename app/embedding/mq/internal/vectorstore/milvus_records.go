package vectorstore

import (
	"context"
	"esx/pkg/vectorprojection"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// validateRecord 拒绝不可追溯（缺模型版本）、维度不符或含非有限值/全零的向量，避免污染召回。
func validateRecord(record Record, expectedDim int) error {
	if record.Revision <= 0 {
		return fmt.Errorf("revision must be positive")
	}
	if record.PostID <= 0 {
		return fmt.Errorf("post ID must be positive")
	}
	if strings.TrimSpace(record.ModelVersion) == "" {
		return fmt.Errorf("model version is required")
	}
	if len(record.ModelVersion) > modelVersionMaxLength {
		return fmt.Errorf("model version exceeds %d bytes", modelVersionMaxLength)
	}
	if record.Dimension != expectedDim {
		return fmt.Errorf("vector dimension metadata mismatch: got %d, want %d", record.Dimension, expectedDim)
	}
	if len(record.Vector) != expectedDim {
		return fmt.Errorf("vector dim mismatch: got %d, want %d", len(record.Vector), expectedDim)
	}
	nonzero := false
	for i, value := range record.Vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("vector contains non-finite value at index %d", i)
		}
		if value != 0 {
			nonzero = true
		}
	}
	if !nonzero {
		return fmt.Errorf("vector is all zero")
	}
	return nil
}

// Upsert 写入单条记录。
func (m *MilvusVectorStore) Upsert(ctx context.Context, record Record) error {
	return m.UpsertBatch(ctx, []Record{record})
}

// UpsertBatch 校验全部记录后一次写入；任一记录无效则整批拒绝。
func (m *MilvusVectorStore) UpsertBatch(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return fmt.Errorf("milvus upsert batch is empty")
	}
	ids := make([]int64, len(records))
	keys := make([]string, len(records))
	revisions := make([]int64, len(records))
	vectors := make([][]float32, len(records))
	versions := make([]string, len(records))
	dimensions := make([]int32, len(records))
	for i, record := range records {
		if err := validateRecord(record, m.dim); err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		ids[i] = record.PostID
		keys[i] = fmt.Sprintf("%d:%d", record.PostID, record.Revision)
		revisions[i] = record.Revision
		vectors[i] = record.Vector
		versions[i] = record.ModelVersion
		dimensions[i] = int32(record.Dimension)
	}
	if _, err := m.cli.Upsert(ctx, m.collection, "",
		entity.NewColumnVarChar("projection_id", keys),
		entity.NewColumnInt64("post_id", ids),
		entity.NewColumnInt64("revision", revisions),
		entity.NewColumnBool("deleted", make([]bool, len(records))),
		entity.NewColumnFloatVector("embedding", m.dim, vectors),
		entity.NewColumnVarChar("model_version", versions),
		entity.NewColumnInt32("dimension", dimensions),
	); err != nil {
		return fmt.Errorf("milvus upsert into %q: %w", m.collection, err)
	}
	return nil
}

// Immutable revision keys remove the remote read/write CAS race: a delayed old
// RPC can add only its own revision, never replace a newer vector or tombstone.
// Readers MUST resolve Latest with strong consistency before using a candidate.
func (m *MilvusVectorStore) Delete(ctx context.Context, postID, revision int64) error {
	if postID <= 0 || revision <= 0 {
		return fmt.Errorf("post ID and revision must be positive")
	}
	// A separate tombstone key wins even if a same-revision upsert races it.
	vector := make([]float32, m.dim)
	vector[0] = 1
	_, err := m.cli.Upsert(ctx, m.collection, "",
		entity.NewColumnVarChar("projection_id", []string{fmt.Sprintf("%d:%d:deleted", postID, revision)}),
		entity.NewColumnInt64("post_id", []int64{postID}), entity.NewColumnInt64("revision", []int64{revision}),
		entity.NewColumnBool("deleted", []bool{true}), entity.NewColumnFloatVector("embedding", m.dim, [][]float32{vector}),
		entity.NewColumnVarChar("model_version", []string{"tombstone"}), entity.NewColumnInt32("dimension", []int32{int32(m.dim)}))
	if err != nil {
		return fmt.Errorf("milvus tombstone: %w", err)
	}
	return nil
}

// CurrentRevision 返回帖子当前已写入的最高修订号。
func (m *MilvusVectorStore) CurrentRevision(ctx context.Context, postID int64) (int64, error) {
	rows, err := vectorprojection.Latest(ctx, m.cli, m.collection, []int64{postID}, false)
	if err != nil {
		return 0, err
	}
	return rows[postID].Revision, nil
}

// Flush 让已写入的数据持久化并可被检索。
func (m *MilvusVectorStore) Flush(ctx context.Context) error {
	if err := m.cli.Flush(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus flush collection %q: %w", m.collection, err)
	}
	return nil
}

// Count 返回集合中的实体数，供重建后核对。
func (m *MilvusVectorStore) Count(ctx context.Context) (int64, error) {
	stats, err := m.cli.GetCollectionStatistics(ctx, m.collection)
	if err != nil {
		return 0, fmt.Errorf("milvus collection statistics for %q: %w", m.collection, err)
	}
	count, err := strconv.ParseInt(stats["row_count"], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("milvus collection %q has invalid row_count %q: %w", m.collection, stats["row_count"], err)
	}
	return count, nil
}
