package vectorstore

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMilvusStoreFailsWhenRequiredServiceIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	store, err := NewMilvusVectorStore(ctx, "127.0.0.1:1", "required_collection", 2)
	if err == nil {
		defer store.Close()
		err = store.OpenCollection(ctx)
	}
	require.Error(t, err)
}

func TestMilvusStartupErrorsAreTheOnlyRetryableConnectionFailures(t *testing.T) {
	t.Parallel()
	retryable := []error{
		errors.New("service unavailable: Milvus Proxy is not ready yet"),
		errors.New("SERVICE UNAVAILABLE"),
	}
	for _, err := range retryable {
		require.True(t, isMilvusStartupError(err), err.Error())
	}
	require.False(t, isMilvusStartupError(errors.New("authentication failed")))
}

func TestValidateRecordRequiresTraceableFiniteVector(t *testing.T) {
	valid := Record{PostID: 1, Vector: []float32{0.1, 0.2}, ModelVersion: "model@sha", Dimension: 2}
	require.NoError(t, validateRecord(valid, 2))

	tests := []struct {
		name   string
		mutate func(*Record)
		want   string
	}{
		{name: "model", mutate: func(r *Record) { r.ModelVersion = "" }, want: "model version"},
		{name: "metadata dimension", mutate: func(r *Record) { r.Dimension = 3 }, want: "metadata mismatch"},
		{name: "vector dimension", mutate: func(r *Record) { r.Vector = []float32{0.1} }, want: "dim mismatch"},
		{name: "zero", mutate: func(r *Record) { r.Vector = []float32{0, 0} }, want: "all zero"},
		{name: "nan", mutate: func(r *Record) { r.Vector[1] = float32(math.NaN()) }, want: "non-finite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := valid
			record.Vector = append([]float32(nil), valid.Vector...)
			tt.mutate(&record)
			err := validateRecord(record, 2)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateSchemaRequiresMetadataAndExpectedDimension(t *testing.T) {
	schema := (&MilvusVectorStore{collection: "target", dim: 4}).schema()
	require.NoError(t, validateSchema(schema, 4))

	missingMetadata := &entity.Schema{Fields: []*entity.Field{
		schema.Fields[0], schema.Fields[1], schema.Fields[3],
	}}
	err := validateSchema(missingMetadata, 4)
	require.ErrorContains(t, err, "model_version")

	err = validateSchema(schema, 8)
	require.ErrorContains(t, err, "embedding dimension")
}
