package event

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func validDecision() ReviewDecidedEvent {
	return ReviewDecidedEvent{
		EventID: 1, EventTime: 1, BizType: ReviewBizAdCreative, ObjectID: 2, Revision: 1, TaskID: 3,
		Purpose: ReviewPurposeRescan, Verdict: ReviewVerdictReject, PolicyCodes: []string{"CONTENT.DECEPTIVE"},
		PolicyVersion: "v1", Source: ReviewSourceMachine, DecidedAt: 1, Interim: true,
	}
}

// ADS-031：暂停结论只用于回扫判定违规。
func TestInterimDecisionIsRescanRejectionOnly(t *testing.T) {
	require.NoError(t, validDecision().Validate())

	approve := validDecision()
	approve.Verdict, approve.PolicyCodes = ReviewVerdictApprove, nil
	require.Error(t, approve.Validate())

	report := validDecision()
	report.Purpose = ReviewPurposeReport
	require.Error(t, report.Validate())

	final := validDecision()
	final.Interim = false
	final.Purpose = ReviewPurposeReport
	require.NoError(t, final.Validate())
}
