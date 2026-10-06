package store

// QuestionOption is one selectable answer of a follow-up question.
type QuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Question is one follow-up question the agent asks before researching.
type Question struct {
	ID        string           `json:"id"`
	Text      string           `json:"text"`
	Selection string           `json:"selection"`
	Options   []QuestionOption `json:"options"`
}

// QuestionAnswer is the user's answer to one question.
type QuestionAnswer struct {
	QuestionID        string   `json:"questionId"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
	Text              string   `json:"text"`
	Disposition       string   `json:"disposition"`
}

// QuestionRequest groups the questions of one tool call; the answer request ID and digest make resubmission idempotent.
type QuestionRequest struct {
	ID              string           `json:"id"`
	RunID           int64            `json:"runId"`
	UserID          int64            `json:"-"`
	CallID          string           `json:"callId"`
	MessageID       int64            `json:"messageId"`
	Status          string           `json:"status"`
	Questions       []Question       `json:"questions"`
	Answers         []QuestionAnswer `json:"answers"`
	DeadlineMs      int64            `json:"deadlineMs"`
	CreatedAtMs     int64            `json:"createdAtMs"`
	AnswerRequestID string           `json:"-"`
	AnswerDigest    string           `json:"-"`
}

// Evidence is an actually retrieved fragment, not a model-generated summary.
type Evidence struct {
	ID            string `json:"id"`
	Handle        string `json:"handle"`
	RunID         int64  `json:"-"`
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	CommentID     string `json:"commentId,omitempty"`
	RetrievedAtMs int64  `json:"retrievedAtMs"`
}

// AnswerCitation links an answer block to evidence fragments of one source.
type AnswerCitation struct {
	Handle      string   `json:"handle"`
	EvidenceIDs []string `json:"evidenceIds"`
}

// AnswerBlock is one paragraph of a structured answer with its citations.
type AnswerBlock struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	Text      string           `json:"text"`
	Citations []AnswerCitation `json:"citations"`
}

// ResearchSource is a source shown under an answer, with the excerpts actually cited.
type ResearchSource struct {
	Handle            string     `json:"handle"`
	Kind              string     `json:"kind"`
	AuthorityID       string     `json:"authorityId"`
	Title             string     `json:"title"`
	Revision          int64      `json:"revision"`
	URL               string     `json:"url"`
	ThumbnailURL      string     `json:"thumbnailUrl,omitempty"`
	Author            string     `json:"author,omitempty"`
	PublishedAtMs     int64      `json:"publishedAtMs,omitempty"`
	Available         bool       `json:"available"`
	UnavailableReason string     `json:"unavailableReason,omitempty"`
	Excerpts          []Evidence `json:"excerpts"`
}

// AnswerPresentation is the structured rendering stored alongside an assistant message.
type AnswerPresentation struct {
	Version   int              `json:"version"`
	MessageID int64            `json:"messageId"`
	RunID     int64            `json:"runId"`
	Blocks    []AnswerBlock    `json:"blocks"`
	Sources   []ResearchSource `json:"sources"`
}
