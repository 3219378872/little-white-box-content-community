package embedder

import "context"

// EmbeddingDim 是默认向量维度，与父 spec §6.4 post_embedding 字段一致。
const EmbeddingDim = 256

// Embedder 把帖子内容转成向量。生产环境应替换为
// Python sentence-transformers gRPC 客户端，初期用 NoopEmbedder 占位。
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NoopEmbedder 产出全零向量，仅用于 Iter 1 起步与离线测试。
// 父 spec §8.2 已声明 Embedding 模型独立部署，本占位实现保留接口契约。
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, EmbeddingDim), nil
}
