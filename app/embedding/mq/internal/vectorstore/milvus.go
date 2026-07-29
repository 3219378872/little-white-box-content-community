package vectorstore

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/grpc"
)

const (
	modelVersionMaxLength       = 256
	defaultMilvusConnectTimeout = 90 * time.Second
	milvusConnectRetryInterval  = 500 * time.Millisecond
)

type MilvusVectorStore struct {
	cli        client.Client
	collection string
	dim        int
}

type MilvusOption func(*milvusOptions)

type milvusOptions struct {
	username string
	password string
	dbName   string
}

func WithMilvusAuth(user, password string) MilvusOption {
	return func(o *milvusOptions) {
		o.username = user
		o.password = password
	}
}

func WithMilvusDatabase(db string) MilvusOption {
	return func(o *milvusOptions) {
		o.dbName = db
	}
}

func NewMilvusVectorStore(ctx context.Context, addr, collection string, dim int, opts ...MilvusOption) (*MilvusVectorStore, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("milvus address is required")
	}
	if strings.TrimSpace(collection) == "" {
		return nil, fmt.Errorf("milvus collection is required")
	}
	if dim <= 0 {
		return nil, fmt.Errorf("milvus vector dimension must be positive")
	}
	o := &milvusOptions{}
	for _, opt := range opts {
		opt(o)
	}
	connectCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		connectCtx, cancel = context.WithTimeout(ctx, defaultMilvusConnectTimeout)
	}
	defer cancel()

	config := client.Config{
		Address: addr, Username: o.username, Password: o.password, DBName: o.dbName,
		DialOptions: []grpc.DialOption{grpc.WithNoProxy()},
	}
	var cli client.Client
	for {
		var err error
		cli, err = client.NewClient(connectCtx, config)
		if err == nil {
			break
		}
		if !isMilvusStartupError(err) {
			return nil, fmt.Errorf("milvus connect: %w", err)
		}
		select {
		case <-connectCtx.Done():
			return nil, fmt.Errorf("milvus connect: %w", err)
		case <-time.After(milvusConnectRetryInterval):
		}
	}
	return &MilvusVectorStore{cli: cli, collection: collection, dim: dim}, nil
}

func isMilvusStartupError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not ready") || strings.Contains(message, "service unavailable")
}

func (m *MilvusVectorStore) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(90 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for {
		_, err := m.cli.HasCollection(ctx, "__embedding_readiness_probe__")
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "not ready") && !strings.Contains(err.Error(), "service unavailable") {
			return fmt.Errorf("milvus readiness probe: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("milvus not ready before deadline: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

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

func (m *MilvusVectorStore) schema() *entity.Schema {
	return &entity.Schema{
		CollectionName: m.collection,
		Description:    "versioned post embeddings for search and recommendation",
		AutoID:         false,
		Fields: []*entity.Field{
			{Name: "post_id", DataType: entity.FieldTypeInt64, PrimaryKey: true, AutoID: false},
			{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": strconv.Itoa(m.dim)}},
			{Name: "model_version", DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": strconv.Itoa(modelVersionMaxLength)}},
			{Name: "dimension", DataType: entity.FieldTypeInt32},
		},
		EnableDynamicField: false,
	}
}

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
	return nil
}

func validateSchema(schema *entity.Schema, expectedDim int) error {
	want := map[string]entity.FieldType{
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
		if field.Name == "post_id" && (!field.PrimaryKey || field.AutoID) {
			return fmt.Errorf("post_id must be a non-auto primary key")
		}
		if field.Name == "embedding" {
			dim, err := strconv.Atoi(field.TypeParams["dim"])
			if err != nil || dim != expectedDim {
				return fmt.Errorf("embedding dimension is %q, want %d", field.TypeParams["dim"], expectedDim)
			}
		}
		if field.Name == "model_version" {
			maxLength, err := strconv.Atoi(field.TypeParams["max_length"])
			if err != nil || maxLength < modelVersionMaxLength {
				return fmt.Errorf("model_version max_length is %q, want at least %d", field.TypeParams["max_length"], modelVersionMaxLength)
			}
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

func validateRecord(record Record, expectedDim int) error {
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

func (m *MilvusVectorStore) Upsert(ctx context.Context, record Record) error {
	return m.UpsertBatch(ctx, []Record{record})
}

func (m *MilvusVectorStore) UpsertBatch(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return fmt.Errorf("milvus upsert batch is empty")
	}
	ids := make([]int64, len(records))
	vectors := make([][]float32, len(records))
	versions := make([]string, len(records))
	dimensions := make([]int32, len(records))
	for i, record := range records {
		if err := validateRecord(record, m.dim); err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		ids[i] = record.PostID
		vectors[i] = record.Vector
		versions[i] = record.ModelVersion
		dimensions[i] = int32(record.Dimension)
	}
	if _, err := m.cli.Upsert(ctx, m.collection, "",
		entity.NewColumnInt64("post_id", ids),
		entity.NewColumnFloatVector("embedding", m.dim, vectors),
		entity.NewColumnVarChar("model_version", versions),
		entity.NewColumnInt32("dimension", dimensions),
	); err != nil {
		return fmt.Errorf("milvus upsert into %q: %w", m.collection, err)
	}
	return nil
}

func (m *MilvusVectorStore) Delete(ctx context.Context, postID int64) error {
	if postID <= 0 {
		return fmt.Errorf("post ID must be positive")
	}
	if err := m.cli.Delete(ctx, m.collection, "", fmt.Sprintf("post_id == %d", postID)); err != nil {
		return fmt.Errorf("milvus delete from %q: %w", m.collection, err)
	}
	return nil
}

func (m *MilvusVectorStore) Flush(ctx context.Context) error {
	if err := m.cli.Flush(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus flush collection %q: %w", m.collection, err)
	}
	return nil
}

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

func (m *MilvusVectorStore) Drop(ctx context.Context) error {
	if err := m.cli.DropCollection(ctx, m.collection); err != nil {
		return fmt.Errorf("milvus drop collection %q: %w", m.collection, err)
	}
	return nil
}

func (m *MilvusVectorStore) Close() error {
	if m == nil || m.cli == nil {
		return nil
	}
	return m.cli.Close()
}
