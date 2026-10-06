package vectorstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// OpenCollection verifies that a configured physical collection or alias already
// exists. Runtime consumers use this path so a missing rebuild is a startup error.
func (m *MilvusVectorStore) OpenCollection(ctx context.Context) error {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	exists, err := m.cli.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("milvus has collection %q: %w", m.collection, err)
	}
	if !exists {
		return fmt.Errorf("milvus collection or alias %q does not exist; run the embedding rebuild first", m.collection)
	}
	if err := m.validateExistingCollection(ctx); err != nil {
		return err
	}
	if err := m.cli.LoadCollection(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus load collection %q: %w", m.collection, err)
	}
	return nil
}

// EnsureCollection creates a versioned rebuild target when absent and validates
// its schema when it already exists.
func (m *MilvusVectorStore) EnsureCollection(ctx context.Context) error {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	exists, err := m.cli.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("milvus has collection %q: %w", m.collection, err)
	}
	if !exists {
		if err := m.cli.CreateCollection(ctx, m.schema(), 2); err != nil {
			return fmt.Errorf("milvus create collection %q: %w", m.collection, err)
		}
		idx, err := entity.NewIndexIvfFlat(entity.L2, 128)
		if err != nil {
			return fmt.Errorf("milvus build index: %w", err)
		}
		if err := m.cli.CreateIndex(ctx, m.collection, "embedding", idx, false); err != nil {
			return fmt.Errorf("milvus create index for %q: %w", m.collection, err)
		}
	} else if err := m.validateExistingCollection(ctx); err != nil {
		return err
	}
	if err := m.cli.LoadCollection(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus load collection %q: %w", m.collection, err)
	}
	return nil
}

// CreateCollection creates an empty rebuild target and refuses to reuse an
// existing collection, preventing an explicit target name from overwriting data.
func (m *MilvusVectorStore) CreateCollection(ctx context.Context) (err error) {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	exists, err := m.cli.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("milvus has collection %q: %w", m.collection, err)
	}
	if exists {
		return fmt.Errorf("milvus rebuild target %q already exists", m.collection)
	}
	if err := m.cli.CreateCollection(ctx, m.schema(), 2); err != nil {
		return fmt.Errorf("milvus create collection %q: %w", m.collection, err)
	}
	created := true
	defer func() {
		if err == nil || !created {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if cleanupErr := m.cli.DropCollection(cleanupCtx, m.collection); cleanupErr != nil {
			err = fmt.Errorf("%w; cleanup partial collection: %v", err, cleanupErr)
		}
	}()
	idx, err := entity.NewIndexIvfFlat(entity.L2, 128)
	if err != nil {
		return fmt.Errorf("milvus build index: %w", err)
	}
	if err := m.cli.CreateIndex(ctx, m.collection, "embedding", idx, false); err != nil {
		return fmt.Errorf("milvus create index for %q: %w", m.collection, err)
	}
	if err := m.cli.LoadCollection(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus load collection %q: %w", m.collection, err)
	}
	created = false
	return nil
}

// schema 是向量集合的期望结构：projection_id 主键由帖子与修订号派生，向量带模型版本与维度元数据。
func (m *MilvusVectorStore) schema() *entity.Schema {
	return &entity.Schema{
		CollectionName: m.collection,
		Description:    "versioned post embeddings for search and recommendation",
		AutoID:         false,
		Fields: []*entity.Field{
			{Name: "projection_id", DataType: entity.FieldTypeVarChar, PrimaryKey: true, AutoID: false, TypeParams: map[string]string{"max_length": "64"}},
			{Name: "post_id", DataType: entity.FieldTypeInt64},
			{Name: "revision", DataType: entity.FieldTypeInt64},
			{Name: "deleted", DataType: entity.FieldTypeBool},
			{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": strconv.Itoa(m.dim)}},
			{Name: "model_version", DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": strconv.Itoa(modelVersionMaxLength)}},
			{Name: "dimension", DataType: entity.FieldTypeInt32},
		},
		EnableDynamicField: false,
	}
}

// validateExistingCollection 校验已有集合结构兼容，并把写入目标固定到其物理集合名。
func (m *MilvusVectorStore) validateExistingCollection(ctx context.Context) error {
	collection, err := m.cli.DescribeCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("milvus describe collection %q: %w", m.collection, err)
	}
	if collection == nil || collection.Schema == nil {
		return fmt.Errorf("milvus collection %q returned no schema", m.collection)
	}
	if err := validateSchema(collection.Schema, m.dim); err != nil {
		return fmt.Errorf("milvus collection %q schema is incompatible: %w", m.collection, err)
	}
	// The SDK's Collection.Name echoes the lookup argument (possibly an
	// alias); Schema.CollectionName is the server's canonical physical name.
	// Pin writes so a delayed RPC from this process cannot target a newly
	// promoted collection after rebuild. Restart consumers after promotion.
	if strings.TrimSpace(collection.Schema.CollectionName) == "" {
		return fmt.Errorf("milvus collection %q has no physical schema name", m.collection)
	}
	m.collection = collection.Schema.CollectionName
	return nil
}

// validateSchema 要求集合包含全部必需字段且类型、主键与长度/维度参数兼容；多余字段不影响。
func validateSchema(schema *entity.Schema, expectedDim int) error {
	want := map[string]entity.FieldType{
		"projection_id": entity.FieldTypeVarChar,
		"revision":      entity.FieldTypeInt64,
		"deleted":       entity.FieldTypeBool,
		"post_id":       entity.FieldTypeInt64,
		"embedding":     entity.FieldTypeFloatVector,
		"model_version": entity.FieldTypeVarChar,
		"dimension":     entity.FieldTypeInt32,
	}
	seen := make(map[string]bool, len(want))
	for _, field := range schema.Fields {
		fieldType, required := want[field.Name]
		if !required {
			continue
		}
		if field.DataType != fieldType {
			return fmt.Errorf("field %s has type %s, want %s", field.Name, field.DataType.Name(), fieldType.Name())
		}
		if err := validateFieldParams(field, expectedDim); err != nil {
			return err
		}
		seen[field.Name] = true
	}
	for name := range want {
		if !seen[name] {
			return fmt.Errorf("required field %s is missing", name)
		}
	}
	return nil
}

// validateFieldParams 检查个别字段的附加约束：主键不能自增且足够长、向量维度一致、模型版本列足够长。
func validateFieldParams(field *entity.Field, expectedDim int) error {
	switch field.Name {
	case "projection_id":
		if !field.PrimaryKey || field.AutoID {
			return fmt.Errorf("projection_id must be a non-auto primary key")
		}
		maxLength, err := strconv.Atoi(field.TypeParams["max_length"])
		if err != nil || maxLength < 64 {
			return fmt.Errorf("projection_id max_length must be at least 64")
		}
	case "embedding":
		dim, err := strconv.Atoi(field.TypeParams["dim"])
		if err != nil || dim != expectedDim {
			return fmt.Errorf("embedding dimension is %q, want %d", field.TypeParams["dim"], expectedDim)
		}
	case "model_version":
		maxLength, err := strconv.Atoi(field.TypeParams["max_length"])
		if err != nil || maxLength < modelVersionMaxLength {
			return fmt.Errorf("model_version max_length is %q, want at least %d", field.TypeParams["max_length"], modelVersionMaxLength)
		}
	}
	return nil
}

// PromoteAlias 把别名切换到当前集合，完成重建后的原子切换。
func (m *MilvusVectorStore) PromoteAlias(ctx context.Context, alias string) error {
	if strings.TrimSpace(alias) == "" {
		return fmt.Errorf("milvus promotion alias is required")
	}
	if alias == m.collection {
		return fmt.Errorf("milvus alias must differ from target collection %q", m.collection)
	}
	exists, err := m.cli.HasCollection(ctx, alias)
	if err != nil {
		return fmt.Errorf("milvus check alias %q: %w", alias, err)
	}
	if exists {
		if err := m.cli.AlterAlias(ctx, m.collection, alias); err != nil {
			return fmt.Errorf("milvus promote %q to alias %q: %w", m.collection, alias, err)
		}
		return nil
	}
	if err := m.cli.CreateAlias(ctx, m.collection, alias); err != nil {
		return fmt.Errorf("milvus create alias %q for %q: %w", alias, m.collection, err)
	}
	return nil
}

// Drop 删除当前集合，用于清理失败的重建目标。
func (m *MilvusVectorStore) Drop(ctx context.Context) error {
	if err := m.cli.DropCollection(ctx, m.collection); err != nil {
		return fmt.Errorf("milvus drop collection %q: %w", m.collection, err)
	}
	return nil
}
