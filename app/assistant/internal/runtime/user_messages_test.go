package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"

	"github.com/stretchr/testify/require"
)

// requireUserMessage asserts err is a business error with the given code and user-facing text.
func requireUserMessage(t *testing.T, err error, code int, message string) {
	t.Helper()
	var biz *errx.BizError
	require.True(t, errors.As(err, &biz), "err=%v", err)
	require.Equal(t, code, biz.Code)
	require.Equal(t, message, biz.Message)
}

func TestValidateAnswersReportsChineseMessages(t *testing.T) {
	single := store.Question{ID: "budget", Selection: "single", Options: []store.QuestionOption{{ID: "low"}, {ID: "high"}}}
	other := store.Question{ID: "city", Selection: "multiple"}
	for _, tc := range []struct {
		name      string
		questions []store.Question
		answers   []store.QuestionAnswer
		message   string
	}{
		{"missing", []store.Question{single}, nil, "每个问题都需要作答或明确跳过"},
		{"duplicate", []store.Question{single, other}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "skipped"}, {QuestionID: "budget", Disposition: "skipped"}}, "同一问题不能重复作答"},
		{"foreign", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "city", Disposition: "skipped"}}, "回答与问题不匹配"},
		{"too long", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "answered", Text: strings.Repeat("字", 2001)}}, "回答总字数不能超过 2000 字"},
		{"empty", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "answered"}}, "回答内容不能为空"},
		{"skip with options", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "unknown", SelectedOptionIDs: []string{"low"}}}, "选择不知道、无偏好或跳过时不能再选选项"},
		{"bad disposition", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "maybe"}}, "回答方式无效"},
		{"two singles", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "answered", SelectedOptionIDs: []string{"low", "high"}}}, "单选题只能选择一个选项"},
		{"bad option", []store.Question{single}, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "answered", SelectedOptionIDs: []string{"mid"}}}, "选项无效或重复"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateAnswers(tc.questions, tc.answers)
			requireUserMessage(t, err, errx.ParamError, tc.message)
		})
	}
}

func TestQuestionFlowConflictsReportChineseMessages(t *testing.T) {
	mem, _, run, _ := questionRun(t)
	ctx := context.Background()
	questions, err := mem.ListQuestions(ctx, run.ID)
	require.NoError(t, err)
	answers := []store.QuestionAnswer{{QuestionID: "budget", Disposition: "unknown"}}

	accept := &Acceptor{Store: mem}
	_, err = accept.Accept(ctx, AcceptInput{UserID: 1, Message: "换个问题", RequestID: "old-client", ConsentOK: true, ConsentVersion: 3, ClientProtocolVersion: 1})
	requireUserMessage(t, err, errx.ParamError, "当前客户端版本过低，请更新后继续")

	_, err = accept.Accept(ctx, AcceptInput{
		UserID: 1, Message: "补充回答", RequestID: "context-active", ConsentOK: true, ConsentVersion: 3, ClientProtocolVersion: 2,
		QuestionContext: &QuestionContext{RunID: run.ID, QuestionRequestID: questions[0].ID, Answers: answers},
	})
	requireUserMessage(t, err, errx.ContentVersionConflict, "原来的回答仍在进行中，请稍后再试")

	_, err = AnswerQuestions(ctx, mem, 1, run.ID, questions[0].ID, "first", answers)
	require.NoError(t, err)
	_, err = AnswerQuestions(ctx, mem, 1, run.ID, questions[0].ID, "second", answers)
	requireUserMessage(t, err, errx.ContentVersionConflict, "这个问题已回答、已过期或已取消")
}

func TestResolveWaitingHidesMissingQuestionDetail(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	run, err := mem.InsertRun(ctx, store.Run{UserID: 1, Status: store.StatusWaitingInput, Phase: store.PhaseWaitingInput, ConsentVersion: 3})
	require.NoError(t, err)
	requireUserMessage(t, ResolveWaiting(ctx, mem, run.ID, store.NowMs()), errx.SystemError, errx.GetMsg(errx.SystemError))
}

func TestCancelledRunEventsCarryChineseText(t *testing.T) {
	ctx := context.Background()
	lastEventText := func(t *testing.T, mem *store.MemoryStore, runID int64) string {
		t.Helper()
		events, err := mem.ListEventsAfter(ctx, runID, 0)
		require.NoError(t, err)
		require.NotEmpty(t, events)
		last := ToPB(events[len(events)-1])
		require.Equal(t, "CANCELLED", last.ErrorCode)
		return last.Text
	}

	mem := store.NewMemoryStore()
	engine, run := newCancelTestEngine(t, mem, nil)
	require.NoError(t, engine.cancel(ctx, run))
	require.Equal(t, "运行已取消", lastEventText(t, mem, run.ID))

	mem = store.NewMemoryStore()
	_, run = newCancelTestEngine(t, mem, nil)
	require.NoError(t, (&Acceptor{Store: mem}).DeleteHistory(ctx, 1))
	require.Equal(t, "运行已取消", lastEventText(t, mem, run.ID))

	require.Equal(t, "运行已取消", cancelledOutcome().payload.Text)
}
