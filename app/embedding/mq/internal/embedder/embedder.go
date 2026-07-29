package embedder

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	embeddingpb "esx/app/embedding/mq/xiaobaihe/embedding/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Metadata struct {
	ModelVersion string
	Dimension    int
}

type Embedding struct {
	Vector []float32
	Metadata
}

type Embedder interface {
	Embed(ctx context.Context, text string) (Embedding, error)
}

type BatchEmbedder interface {
	Embedder
	EmbedBatch(ctx context.Context, texts []string) ([]Embedding, error)
}

type ClientConfig struct {
	Address              string
	ExpectedModelVersion string
	ExpectedDimension    int
	Timeout              time.Duration
	MaxTextBytes         int
	MaxBatchSize         int
	MaxBatchBytes        int
}

type GRPCEmbedder struct {
	client embeddingpb.EmbeddingServiceClient
	conn   *grpc.ClientConn
	cfg    ClientConfig
}

func NewGRPCEmbedder(ctx context.Context, cfg ClientConfig) (*GRPCEmbedder, error) {
	if err := validateConfig(cfg, true); err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithNoProxy(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect embedding service: %w", err)
	}
	emb, err := NewGRPCEmbedderWithClient(ctx, embeddingpb.NewEmbeddingServiceClient(conn), cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	emb.conn = conn
	return emb, nil
}

func NewGRPCEmbedderWithClient(ctx context.Context, client embeddingpb.EmbeddingServiceClient, cfg ClientConfig) (*GRPCEmbedder, error) {
	if client == nil {
		return nil, fmt.Errorf("embedding gRPC client is required")
	}
	if err := validateConfig(cfg, false); err != nil {
		return nil, err
	}
	emb := &GRPCEmbedder{client: client, cfg: cfg}
	if _, err := emb.Health(ctx); err != nil {
		return nil, fmt.Errorf("embedding service health check: %w", err)
	}
	return emb, nil
}

func validateConfig(cfg ClientConfig, requireAddress bool) error {
	if requireAddress && strings.TrimSpace(cfg.Address) == "" {
		return fmt.Errorf("embedding service address is required")
	}
	if strings.TrimSpace(cfg.ExpectedModelVersion) == "" {
		return fmt.Errorf("expected embedding model version is required")
	}
	if cfg.ExpectedDimension <= 0 {
		return fmt.Errorf("expected embedding dimension must be positive")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("embedding timeout must be positive")
	}
	if cfg.MaxTextBytes <= 0 {
		return fmt.Errorf("embedding max text bytes must be positive")
	}
	if cfg.MaxBatchSize <= 0 {
		return fmt.Errorf("embedding max batch size must be positive")
	}
	if cfg.MaxBatchBytes < cfg.MaxTextBytes {
		return fmt.Errorf("embedding max batch bytes must be at least max text bytes")
	}
	return nil
}

func (e *GRPCEmbedder) Health(ctx context.Context) (Metadata, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()
	resp, err := e.client.Health(callCtx, &embeddingpb.EmbeddingHealthReq{})
	if err != nil {
		return Metadata{}, fmt.Errorf("health RPC: %w", err)
	}
	if resp == nil {
		return Metadata{}, fmt.Errorf("health RPC returned nil response")
	}
	if !resp.Ready {
		return Metadata{}, fmt.Errorf("embedding model is not ready")
	}
	metadata := Metadata{ModelVersion: resp.ModelVersion, Dimension: int(resp.Dimension)}
	if err := e.validateMetadata(metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func (e *GRPCEmbedder) Embed(ctx context.Context, text string) (Embedding, error) {
	if err := e.validateTexts([]string{text}); err != nil {
		return Embedding{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()
	resp, err := e.client.Embed(callCtx, &embeddingpb.EmbedReq{Text: text})
	if err != nil {
		return Embedding{}, fmt.Errorf("embed RPC: %w", err)
	}
	if resp == nil {
		return Embedding{}, fmt.Errorf("embed RPC returned nil response")
	}
	result := Embedding{
		Vector: resp.Vector,
		Metadata: Metadata{
			ModelVersion: resp.ModelVersion,
			Dimension:    int(resp.Dimension),
		},
	}
	if err := e.validateEmbedding(result); err != nil {
		return Embedding{}, err
	}
	return result, nil
}

func (e *GRPCEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]Embedding, error) {
	if err := e.validateTexts(texts); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()
	resp, err := e.client.EmbedBatch(callCtx, &embeddingpb.EmbedBatchReq{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("embed batch RPC: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("embed batch RPC returned nil response")
	}
	if len(resp.Items) != len(texts) {
		return nil, fmt.Errorf("embed batch item count mismatch: got %d, want %d", len(resp.Items), len(texts))
	}
	metadata := Metadata{ModelVersion: resp.ModelVersion, Dimension: int(resp.Dimension)}
	if err := e.validateMetadata(metadata); err != nil {
		return nil, err
	}
	results := make([]Embedding, len(resp.Items))
	for i, item := range resp.Items {
		if item == nil {
			return nil, fmt.Errorf("embed batch item %d is nil", i)
		}
		results[i] = Embedding{Vector: item.Vector, Metadata: metadata}
		if err := e.validateEmbedding(results[i]); err != nil {
			return nil, fmt.Errorf("embed batch item %d: %w", i, err)
		}
	}
	return results, nil
}

func (e *GRPCEmbedder) validateTexts(texts []string) error {
	if len(texts) == 0 {
		return fmt.Errorf("embedding input is empty")
	}
	if len(texts) > e.cfg.MaxBatchSize {
		return fmt.Errorf("embedding batch exceeds limit: got %d, max %d", len(texts), e.cfg.MaxBatchSize)
	}
	totalBytes := 0
	for i, text := range texts {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("embedding input %d is blank", i)
		}
		textBytes := len([]byte(text))
		if textBytes > e.cfg.MaxTextBytes {
			return fmt.Errorf("embedding input %d exceeds byte limit: got %d, max %d", i, textBytes, e.cfg.MaxTextBytes)
		}
		totalBytes += textBytes
	}
	if totalBytes > e.cfg.MaxBatchBytes {
		return fmt.Errorf("embedding batch exceeds byte limit: got %d, max %d", totalBytes, e.cfg.MaxBatchBytes)
	}
	return nil
}

func (e *GRPCEmbedder) validateMetadata(metadata Metadata) error {
	if strings.TrimSpace(metadata.ModelVersion) == "" {
		return fmt.Errorf("embedding response has empty model version")
	}
	if metadata.ModelVersion != e.cfg.ExpectedModelVersion {
		return fmt.Errorf("embedding model version mismatch: got %q, want %q", metadata.ModelVersion, e.cfg.ExpectedModelVersion)
	}
	if metadata.Dimension != e.cfg.ExpectedDimension {
		return fmt.Errorf("embedding dimension metadata mismatch: got %d, want %d", metadata.Dimension, e.cfg.ExpectedDimension)
	}
	return nil
}

func (e *GRPCEmbedder) validateEmbedding(result Embedding) error {
	if err := e.validateMetadata(result.Metadata); err != nil {
		return err
	}
	if len(result.Vector) == 0 {
		return fmt.Errorf("embedding response has empty vector")
	}
	if len(result.Vector) != result.Dimension {
		return fmt.Errorf("embedding vector dimension mismatch: got %d values, metadata says %d", len(result.Vector), result.Dimension)
	}
	nonzero := false
	for i, value := range result.Vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("embedding vector contains non-finite value at index %d", i)
		}
		if value != 0 {
			nonzero = true
		}
	}
	if !nonzero {
		return fmt.Errorf("embedding vector is all zero")
	}
	return nil
}

func (e *GRPCEmbedder) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}
