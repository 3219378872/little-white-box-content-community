package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

// researchDefinitions 声明检索回答流程的三个工具：追问、分页读取来源证据、发布带引用的回答。
func researchDefinitions(clients Clients) []Definition {
	text := map[string]any{"type": "string"}
	option := objectSchema(map[string]any{"id": text, "label": text}, []string{"id", "label"})
	question := objectSchema(map[string]any{
		"id": text, "text": text, "selection": map[string]any{"type": "string", "enum": []string{"single", "multiple"}},
		"options": map[string]any{"type": "array", "minItems": 2, "maxItems": 8, "items": option},
	}, []string{"id", "text", "selection", "options"})
	citation := objectSchema(map[string]any{"handle": text, "evidenceIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": text}}, []string{"handle", "evidenceIds"})
	block := objectSchema(map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"fact", "experience", "inference", "context", "limitation"}},
		"text": text, "citations": map[string]any{"type": "array", "maxItems": 10, "items": citation},
	}, []string{"kind", "text", "citations"})
	return []Definition{
		{Name: AskQuestions, Description: "仅询问显著影响检索的未知条件，每批最多三问。用户可补充文字、未知、无偏好、跳过或先搜索。调用后持久等待真实答案，不作为授权或删除确认。", Parameters: objectSchema(map[string]any{"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": question}}, []string{"questions"}), executor: askQuestionsExecutor},
		{Name: ReadSource, Description: "分页读取本 run 来源的实际证据片段。cursor 为字符偏移，默认零；网页只能读取已取得的搜索摘录，不代表取得全文。", Parameters: objectSchema(map[string]any{"handle": text, "cursor": map[string]any{"type": "integer", "minimum": 0}}, []string{"handle"}), executor: readSourceExecutor(clients)},
		{Name: PublishAnswer, Description: "一次发布完整检索回答。fact/experience/inference 每段必须关联实际来源 handle 和 evidenceIds。context 仅用于用户自述/已知条件，limitation 仅说明检索缺口。不得把资料事实标成 context 逃避引用。成功后本 run 结束。", Parameters: objectSchema(map[string]any{"blocks": map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": block}}, []string{"blocks"}), executor: publishAnswerExecutor(clients)},
	}
}

// ForClient 按客户端协议版本裁剪工具集；支持 publish_answer 的 v2 客户端不再暴露旧的 present_sources。
func ForClient(registry *Registry, version int) *Registry {
	if registry == nil {
		return nil
	}
	var names []string
	for _, def := range registry.Definitions() {
		if version >= 2 && registry.Has(PublishAnswer) && def.Name == PresentSources {
			continue
		}
		meta, ok := registry.Metadata(def.Name)
		if ok && meta.MinClientProtocol <= version && def.MinClientProtocol <= version {
			names = append(names, def.Name)
		}
	}
	return registry.Restrict(names)
}

// askQuestionsExecutor 只允许用户发起的 v2 run 追问；问题记录到会话后 run 挂起等待真实回答，
// 问题 ID 由 run 与 callID 派生，重放同一调用得到相同 ID。
func askQuestionsExecutor(_ context.Context, session *Session, callID, argsJSON string) (string, []store.SourceRef, error) {
	var args struct {
		Questions []store.Question `json:"questions"`
	}
	if err := strictUnmarshal(argsJSON, &args); err != nil {
		return "", nil, errx.New(errx.ParamError, "invalid questions")
	}
	if session == nil || session.Source != store.SourceUser || session.ClientProtocolVersion < 2 {
		return "", nil, errx.NewWithCode(errx.PermissionDenied)
	}
	if err := ValidateQuestions(args.Questions); err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%s", session.RunID, callID)))
	session.Question = &store.QuestionRequest{ID: "q_" + hex.EncodeToString(digest[:16]), RunID: session.RunID, UserID: session.UserID, CallID: callID, Status: "pending", Questions: args.Questions}
	return "等待用户回答。", nil, nil
}

// ValidateQuestions 限制每批 1～3 问、每问 2～8 个选项，ID 唯一且文本长度有界。
func ValidateQuestions(questions []store.Question) error {
	if len(questions) < 1 || len(questions) > 3 {
		return errx.New(errx.ParamError, "questions must contain 1 to 3 items")
	}
	ids := map[string]bool{}
	for _, q := range questions {
		if q.ID == "" || len(q.ID) > 64 || ids[q.ID] || strings.TrimSpace(q.Text) == "" || utf8.RuneCountInString(q.Text) > 300 ||
			(q.Selection != "single" && q.Selection != "multiple") || len(q.Options) < 2 || len(q.Options) > 8 {
			return errx.New(errx.ParamError, "invalid question")
		}
		ids[q.ID] = true
		options := map[string]bool{}
		for _, o := range q.Options {
			if o.ID == "" || len(o.ID) > 64 || options[o.ID] || strings.TrimSpace(o.Label) == "" || utf8.RuneCountInString(o.Label) > 200 {
				return errx.New(errx.ParamError, "invalid question option")
			}
			options[o.ID] = true
		}
	}
	return nil
}
