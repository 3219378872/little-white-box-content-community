package store

type QuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Question struct {
	ID        string           `json:"id"`
	Text      string           `json:"text"`
	Selection string           `json:"selection"`
	Options   []QuestionOption `json:"options"`
}

type QuestionAnswer struct {
	QuestionID        string   `json:"questionId"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
	Text              string   `json:"text"`
	Disposition       string   `json:"disposition"`
}

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

type AnswerCitation struct {
	Handle      string   `json:"handle"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type AnswerBlock struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	Text      string           `json:"text"`
	Citations []AnswerCitation `json:"citations"`
}

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

type AnswerPresentation struct {
	Version   int              `json:"version"`
	MessageID int64            `json:"messageId"`
	RunID     int64            `json:"runId"`
	Blocks    []AnswerBlock    `json:"blocks"`
	Sources   []ResearchSource `json:"sources"`
}
