package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

const (
	requestIDHeader     = "X-Request-ID"
	traceIDHeader       = "X-Trace-ID"
	clientVersionHeader = "X-Client-Version"
)

// behaviorMetadataKey 是行为请求元数据在 ctx 中的私有键。
type behaviorMetadataKey struct{}

// BehaviorRequestMetadata 是网关为行为事件补充的请求侧信息，下游用于去重与风控。
type BehaviorRequestMetadata struct {
	ClientIP      string
	UserAgent     string
	ClientVersion string
	TraceID       string
}

// BehaviorAcceptedMiddleware 为行为上报补齐请求元数据，并把成功响应改写为 202 Accepted。
type BehaviorAcceptedMiddleware struct {
}

// NewBehaviorAcceptedMiddleware 创建行为上报中间件。
func NewBehaviorAcceptedMiddleware() *BehaviorAcceptedMiddleware {
	return &BehaviorAcceptedMiddleware{}
}

// BehaviorRequestMetadataFromContext 读取中间件写入的元数据；未经过中间件时返回零值。
func BehaviorRequestMetadataFromContext(ctx context.Context) BehaviorRequestMetadata {
	metadata, _ := ctx.Value(behaviorMetadataKey{}).(BehaviorRequestMetadata)
	return metadata
}

// newRequestID 生成随机请求 ID；随机源失败时退回固定值，不阻断请求。
func newRequestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-request"
	}
	return hex.EncodeToString(id[:])
}
