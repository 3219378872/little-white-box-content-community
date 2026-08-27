// Package agent 编排 Assistant Agent 模式（SPEC-assistant-agent-mode）。
// Runner 抽象隔离编排引擎：默认实现基于 openai-go 的 chat completions +
// function calling，未来可替换其他 agent SDK 而不影响工具层与事件层。
package agent

import (
	"context"
	"errors"

	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

var (
	// ErrBudgetExhausted 达到硬上限后强制收尾仍失败（AGNT-031）。
	ErrBudgetExhausted = errors.New("agent turn budget exhausted")
	// ErrLLMUnavailable 模型调用在收尾前失败且无法降级（AGNT-061）。
	ErrLLMUnavailable = errors.New("agent LLM unavailable")
	// ErrConfirmExpired 确认等待超时或凭据已失效（AGNT-021）。
	ErrConfirmExpired = errors.New("agent confirmation expired or unknown")
)

// Attachment 是本会话上传、可供写帖工具引用的图片（AGNT-013/040）。
type Attachment struct {
	MediaID int64
	URL     string
}

// Budget 是单轮执行的步数与调用预算（AGNT-030~033）。
type Budget struct {
	MaxStepsSoft        int   // 软上限：超过后工具结果注入剩余轮数通知
	MaxStepsHard        int   // 硬上限：达到后剥离工具强制收尾
	MaxToolCallsPerTurn int   // 单轮工具调用总次数上限
	StepTimeout         int64 // 毫秒；单次模型请求预算
	TurnTimeout         int64 // 毫秒；整轮预算
	ConfirmTimeout      int64 // 秒；高危操作等待用户确认的上限
}

// EmitFunc 把 TOOL_CALL / CONFIRM_REQUIRED 事件实时推给流；返回 error 表示
// 传输已断开，Runner 应中止并透传该错误。
type EmitFunc func(event *pb.ChatEvent) error

// Session 是单轮 Agent 执行的输入与运行时状态。消息历史由 Runner 内部持有；
// Sources 在工具执行过程中累积，最终随 Result 返回供引用校验与落库复用。
type Session struct {
	UserID         int64
	ConversationID string
	RequestID      string
	UserMessage    string
	Attachments    []Attachment
	SystemPrompt   string
	Budget         Budget
	Emit           EmitFunc

	// Tools 与 Confirms 由 svc 装配；Tools 为 nil 时 Run 返回错误。
	Tools    *ToolRegistry
	Confirms ConfirmBroker

	ConsentVersion int32 // AGNT-007：低于当前披露版本时裁剪新分组
	ContextPostID  int64
	Plan           QueryPlan

	sources []tool.Source
}

// QueryPlan 是 Intent Router 的结构化产物，供 Context / Planner 使用。
type QueryPlan struct {
	Intent     string
	EntityType string
	EntityText string
	TimeRange  string
}

func (s *Session) addSources(sources []tool.Source) {
	for _, source := range sources {
		if source.Type == "" || source.ID == "" {
			continue
		}
		s.sources = append(s.sources, source)
	}
}

// Result 是一轮 Agent 执行的产物；Text 为最终回答，Sources 为全部工具来源。
type Result struct {
	Text    string
	Sources []tool.Source
}

// Runner 以会话为粒度执行一轮 Agent 循环。实现必须：
//   - 通过 Session.Emit 实时上报工具事件（AGNT-060）；
//   - 高危工具经 Confirms 走逐次确认（AGNT-020~022）；
//   - 遵守 Budget 的软/硬上限语义（AGNT-030/031）；
//   - 不自行执行任何未经工具层的副作用。
type Runner interface {
	Run(ctx context.Context, session *Session) (*Result, error)
}
