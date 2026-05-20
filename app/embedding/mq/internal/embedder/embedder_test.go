package embedder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopEmbedder_ProducesZeroVector(t *testing.T) {
	v, err := NoopEmbedder{}.Embed(context.Background(), "anything")
	require.NoError(t, err)
	assert.Len(t, v, EmbeddingDim)
	for _, f := range v {
		assert.Equal(t, float32(0), f)
	}
}
