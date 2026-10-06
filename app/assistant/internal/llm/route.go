package llm

import (
	"context"
	"fmt"
	"strings"

	"esx/app/assistant/internal/prompt"
)

// capabilityClient 是能报告路由、模型、边界与流式能力的客户端。
type capabilityClient interface {
	RouteID() string
	ModelName() string
	Boundary() string
	SupportsStreaming() bool
}

// fallbackCapabilityClient 是带备用路由的客户端（韧性包装）。
type fallbackCapabilityClient interface {
	FallbackRouteIDs() []string
}

// Capability 汇总客户端的能力，冻结到 run 中，保证同一 run 的后续轮次使用兼容的路由。
func Capability(client Client) prompt.ProviderCapability {
	if client == nil {
		return prompt.ProviderCapability{}
	}
	capability := prompt.ProviderCapability{
		RouteID: "primary", WireAPI: client.WireAPI(), ContextTokens: client.ContextWindowTokens(),
		MaxOutputTokens: client.MaxOutputTokens(), Tools: client.SupportsTools(),
	}
	if detailed, ok := client.(capabilityClient); ok {
		capability.RouteID = detailed.RouteID()
		capability.Model = detailed.ModelName()
		capability.Boundary = detailed.Boundary()
		capability.Streaming = detailed.SupportsStreaming()
	}
	if fallbacks, ok := client.(fallbackCapabilityClient); ok {
		capability.FallbackRouteIDs = append([]string(nil), fallbacks.FallbackRouteIDs()...)
	}
	return capability
}

// RouteSelector 按路由 ID 选择客户端（允许回退到兼容路由）。
type RouteSelector interface {
	ForRoute(routeID string) (Client, bool)
}

// exactRouteSelector 只返回完全同名的路由。
type exactRouteSelector interface {
	ExactRoute(routeID string) (Client, bool)
}

// capabilityRouteSelector 按冻结能力选择路由。
type capabilityRouteSelector interface {
	ForCapability(capability prompt.ProviderCapability) (Client, bool)
}

// SelectRoute 按路由 ID 选择客户端；单路由客户端只匹配自身。
func SelectRoute(client Client, routeID string) (Client, bool) {
	if client == nil {
		return nil, false
	}
	if selector, ok := client.(RouteSelector); ok {
		return selector.ForRoute(routeID)
	}
	capability := Capability(client)
	return client, routeID == "" || capability.RouteID == routeID
}

// SelectExactRoute 只接受完全同名的路由。
func SelectExactRoute(client Client, routeID string) (Client, bool) {
	if client == nil {
		return nil, false
	}
	if selector, ok := client.(exactRouteSelector); ok {
		return selector.ExactRoute(routeID)
	}
	capability := Capability(client)
	return client, routeID == "" || capability.RouteID == routeID
}

// SelectCapability 选出满足 run 冻结能力的客户端，并包装成对外报告冻结能力的客户端，
// 避免路由配置变化后同一 run 中途切换协议、模型或缩小上下文。
func SelectCapability(client Client, frozen prompt.ProviderCapability) (Client, bool) {
	if client == nil || strings.TrimSpace(frozen.RouteID) == "" {
		return nil, false
	}
	var selected Client
	var ok bool
	if selector, selectable := client.(capabilityRouteSelector); selectable {
		selected, ok = selector.ForCapability(frozen)
	} else {
		selected, ok = SelectRoute(client, frozen.RouteID)
	}
	if !ok || !supportsFrozenCapability(selected, frozen) {
		return nil, false
	}
	bound := &frozenClient{base: selected, capability: frozen}
	if frozen.Streaming {
		return &frozenStreamingClient{frozenClient: bound}, true
	}
	return bound, true
}

// supportsFrozenCapability 要求实际能力不弱于冻结能力：同一路由、协议、模型与边界，窗口与输出上限不缩小。
func supportsFrozenCapability(client Client, frozen prompt.ProviderCapability) bool {
	actual := Capability(client)
	if actual.RouteID != frozen.RouteID || (frozen.Tools && !actual.Tools) ||
		(frozen.Streaming && !actual.Streaming) {
		return false
	}
	if frozen.WireAPI != "" && actual.WireAPI != frozen.WireAPI {
		return false
	}
	if frozen.Model != "" && actual.Model != frozen.Model {
		return false
	}
	if strings.TrimSpace(actual.Boundary) != strings.TrimSpace(frozen.Boundary) {
		return false
	}
	if frozen.ContextTokens > 0 && actual.ContextTokens < frozen.ContextTokens {
		return false
	}
	return frozen.MaxOutputTokens <= 0 || actual.MaxOutputTokens >= frozen.MaxOutputTokens
}

// frozenClient 转发调用，但对外报告冻结时的能力。
type frozenClient struct {
	base       Client
	capability prompt.ProviderCapability
}

// Complete 直接转发给实际路由。
func (c *frozenClient) Complete(ctx context.Context, req Request) (Result, error) {
	return c.base.Complete(ctx, req)
}

// SupportsTools 报告冻结时的工具能力。
func (c *frozenClient) SupportsTools() bool { return c.capability.Tools }

// WireAPI 报告冻结时的协议。
func (c *frozenClient) WireAPI() string { return c.capability.WireAPI }

// MaxOutputTokens 报告冻结时的单次输出上限。
func (c *frozenClient) MaxOutputTokens() int { return c.capability.MaxOutputTokens }

// ContextWindowTokens 报告冻结时的上下文窗口，压缩阈值据此计算。
func (c *frozenClient) ContextWindowTokens() int { return c.capability.ContextTokens }

// RouteID 报告冻结时的路由。
func (c *frozenClient) RouteID() string { return c.capability.RouteID }

// ModelName 报告冻结时的模型。
func (c *frozenClient) ModelName() string { return c.capability.Model }

// Boundary 报告冻结时的数据边界。
func (c *frozenClient) Boundary() string { return c.capability.Boundary }

// SupportsStreaming 为 false：冻结时不支持流式的会话保持非流式。
func (*frozenClient) SupportsStreaming() bool { return false }

// frozenStreamingClient 是冻结时支持流式的路由。
type frozenStreamingClient struct {
	*frozenClient
}

// CompleteStream 转发流式调用；实际路由已不支持流式时报错而不是静默降级。
func (c *frozenStreamingClient) CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	stream, ok := c.base.(StreamingClient)
	if !ok {
		return Result{}, fmt.Errorf("frozen assistant LLM route no longer supports streaming")
	}
	return stream.CompleteStream(ctx, req, emit)
}

// SupportsStreaming 为 true。
func (*frozenStreamingClient) SupportsStreaming() bool { return true }
