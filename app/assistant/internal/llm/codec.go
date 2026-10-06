package llm

import (
	"encoding/json"
	"net/http"

	"esx/app/assistant/internal/prompt"
)

// wireCodec 封装一种上游协议的请求编码、响应解码与流式事件消费；HTTPClient 只负责传输。
type wireCodec interface {
	// endpointSuffix 是基础地址只给到根路径或 /v1 时补全的接口路径。
	endpointSuffix() string
	// setHeaders 设置协议特有的请求头。
	setHeaders(header http.Header)
	// encode 编码请求体；messages 已包含收敛提示。
	encode(messages []prompt.Turn, req Request, maxTokens int, stream bool) ([]byte, error)
	// decode 解码一次完整（非流式）响应。
	decode(raw []byte) (Result, error)
	// consumeEvent 消费一条 SSE 事件并更新流状态，可见文本经 emit 推送。
	consumeEvent(event string, data []byte, state *streamState, emit func(Delta) error) error
}

// newWireCodec 按配置的协议返回编解码器；cfg 提供模型名与计价，需在客户端生命周期内保持有效。
func newWireCodec(cfg *Config) wireCodec {
	if cfg.WireAPI == WireAPIResponses {
		return responsesCodec{cfg: cfg}
	}
	return chatCodec{cfg: cfg}
}

// 两种协议的编解码器都必须实现完整接口。
var (
	_ wireCodec = chatCodec{}
	_ wireCodec = responsesCodec{}
)

// marshalBody 是两种协议共用的 JSON 编码。
func marshalBody(body map[string]any) ([]byte, error) {
	return json.Marshal(body)
}
