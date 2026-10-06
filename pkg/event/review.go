package event

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 审核平台按业务类型接入（SPEC-review-platform）；消息 tag 即业务类型。
const (
	ReviewBizAdCreative              = "ad_creative"
	ReviewBizAdvertiserQualification = "advertiser_qualification"
)

// 审核目的：同一对象 revision 的首次送审只有一条 initial 任务，其余目的各自成任务。
const (
	ReviewPurposeInitial = "initial"
	ReviewPurposeQA      = "qa"
	ReviewPurposeAppeal  = "appeal"
	ReviewPurposeReport  = "report"
	ReviewPurposeRescan  = "rescan"
)

const (
	ReviewVerdictApprove = "approve"
	ReviewVerdictReject  = "reject"
)

// 结论来源（RVW-004）。
const (
	ReviewSourceMachine = "machine"
	ReviewSourceHuman   = "human"
	ReviewSourceQA      = "qa"
)

// ReviewMedia 是快照中的一个素材引用；审核只看内容哈希，不看可变地址。
type ReviewMedia struct {
	MediaID int64  `json:"mediaId"`
	SHA256  string `json:"sha256"`
	Kind    string `json:"kind,omitempty"`
}

// ReviewQualification 是资质对象快照中的一份行业资质。
type ReviewQualification struct {
	QualificationID int64  `json:"qualificationId"`
	Market          string `json:"market"`
	Industry        string `json:"industry"`
	DocumentMediaID int64  `json:"documentMediaId"`
	ValidUntilMs    int64  `json:"validUntilMs"`
}

// ReviewSnapshot 是送审时冻结的内容（RVW-002）。字段对所有业务类型通用；
// 文案按名称放在 Texts，便于规则与召回统一处理。
type ReviewSnapshot struct {
	Texts          map[string]string     `json:"texts"`
	LandingURL     string                `json:"landingUrl,omitempty"`
	Media          []ReviewMedia         `json:"media,omitempty"`
	Market         string                `json:"market"`
	Language       string                `json:"language"`
	Industry       string                `json:"industry,omitempty"`
	SubmitterID    int64                 `json:"submitterId"`
	Qualifications []ReviewQualification `json:"qualifications,omitempty"`
}

// ReviewSubmittedEvent 由业务方在写业务状态的同一事务内经 outbox 发出。
type ReviewSubmittedEvent struct {
	EventID     int64          `json:"eventId"`
	EventTime   int64          `json:"eventTime"`
	BizType     string         `json:"bizType"`
	ObjectID    int64          `json:"objectId"`
	Revision    int64          `json:"revision"`
	Purpose     string         `json:"purpose"`
	PurposeKey  string         `json:"purposeKey,omitempty"`
	SubmittedAt int64          `json:"submittedAt"`
	Priority    int32          `json:"priority,omitempty"`
	Snapshot    ReviewSnapshot `json:"snapshot"`
}

// Validate 检查送审事件的业务类型、对象修订与送审目的。
func (e ReviewSubmittedEvent) Validate() error {
	if !IsReviewBizType(e.BizType) {
		return fmt.Errorf("event: unsupported review biz type %q", e.BizType)
	}
	if e.ObjectID <= 0 || e.Revision <= 0 {
		return fmt.Errorf("event: review object id and revision are required")
	}
	if !isReviewPurpose(e.Purpose) {
		return fmt.Errorf("event: unsupported review purpose %q", e.Purpose)
	}
	if e.SubmittedAt <= 0 {
		return fmt.Errorf("event: review submittedAt is required")
	}
	if strings.TrimSpace(e.Snapshot.Market) == "" || e.Snapshot.SubmitterID <= 0 {
		return fmt.Errorf("event: review snapshot market and submitter are required")
	}
	return nil
}

// MarshalPayload 编码为 outbox 消息体。
func (e ReviewSubmittedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

// ReviewDecidedEvent 是结论下发事件（RVW-040）；业务方按（objectId, revision）CAS 应用。
//
// Interim 为 true 的是回扫机审判定违规时的暂停结论（ADS-031）：业务方先停投，最终结论由同一任务
// 的人审给出。暂停结论不写 review_decision，只经事件下发。
type ReviewDecidedEvent struct {
	EventID       int64    `json:"eventId"`
	EventTime     int64    `json:"eventTime"`
	BizType       string   `json:"bizType"`
	ObjectID      int64    `json:"objectId"`
	Revision      int64    `json:"revision"`
	TaskID        int64    `json:"taskId"`
	Purpose       string   `json:"purpose"`
	PurposeKey    string   `json:"purposeKey,omitempty"`
	Verdict       string   `json:"verdict"`
	PolicyCodes   []string `json:"policyCodes"`
	PolicyVersion string   `json:"policyVersion"`
	Source        string   `json:"source"`
	DecidedAt     int64    `json:"decidedAt"`
	Interim       bool     `json:"interim,omitempty"`
}

// Validate 检查审核结论事件的对象、任务身份与结论。
func (e ReviewDecidedEvent) Validate() error {
	if !IsReviewBizType(e.BizType) {
		return fmt.Errorf("event: unsupported review biz type %q", e.BizType)
	}
	if e.ObjectID <= 0 || e.Revision <= 0 || e.TaskID <= 0 {
		return fmt.Errorf("event: review decision identity is required")
	}
	if !isReviewPurpose(e.Purpose) {
		return fmt.Errorf("event: unsupported review purpose %q", e.Purpose)
	}
	switch e.Verdict {
	case ReviewVerdictApprove:
	case ReviewVerdictReject:
		if len(e.PolicyCodes) == 0 {
			return fmt.Errorf("event: rejection requires at least one policy code")
		}
	default:
		return fmt.Errorf("event: unsupported verdict %q", e.Verdict)
	}
	if e.PolicyVersion == "" || e.Source == "" || e.DecidedAt <= 0 {
		return fmt.Errorf("event: review decision metadata is required")
	}
	if e.Interim && (e.Purpose != ReviewPurposeRescan || e.Verdict != ReviewVerdictReject) {
		return fmt.Errorf("event: interim decisions are rescan rejections only")
	}
	return nil
}

// MarshalPayload 编码为 outbox 消息体。
func (e ReviewDecidedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

// IsReviewBizType 判断是否为审核平台支持的业务类型（广告创意、广告主资质）。
func IsReviewBizType(bizType string) bool {
	return bizType == ReviewBizAdCreative || bizType == ReviewBizAdvertiserQualification
}

// isReviewPurpose 判断是否为已定义的送审目的。
func isReviewPurpose(purpose string) bool {
	switch purpose {
	case ReviewPurposeInitial, ReviewPurposeQA, ReviewPurposeAppeal, ReviewPurposeReport, ReviewPurposeRescan:
		return true
	}
	return false
}
