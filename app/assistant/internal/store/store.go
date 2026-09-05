package store

import "context"

// Store is the MySQL authority for Assistant threads, runs, events and side tables.
type Store interface {
	Transact(ctx context.Context, fn func(ctx context.Context, tx Store) error) error
	RunStep(ctx context.Context, fence LeaseFence, fn func(ctx context.Context, tx Store) error) error

	LockThread(ctx context.Context, userID int64) (*Thread, error)
	GetThread(ctx context.Context, userID int64) (*Thread, error)
	SaveThread(ctx context.Context, thread Thread) error

	CreateSession(ctx context.Context, session Session) (Session, error)
	GetSession(ctx context.Context, id int64) (*Session, error)
	UpdateSession(ctx context.Context, session Session) error
	CloseSession(ctx context.Context, id int64, closedAtMs int64) error

	InsertMessage(ctx context.Context, msg Message) (Message, error)
	GetMessage(ctx context.Context, userID, id int64) (*Message, error)
	ListMessages(ctx context.Context, userID, sessionID, beforeID, afterID int64, limit int) ([]Message, error)
	ListSessionMessages(ctx context.Context, userID, sessionID int64, includeHidden bool) ([]Message, error)
	SoftDeleteMessages(ctx context.Context, userID, deletedAtMs int64) (ids []int64, err error)
	MarkMessagesRead(ctx context.Context, userID int64) error
	MarkMessagesCompacted(ctx context.Context, ids []int64) error
	GetMessagesByIDs(ctx context.Context, userID int64, ids []int64) ([]Message, error)
	ListHistoryAround(ctx context.Context, userID, messageID int64, before, after int, cutoffMs int64, excludeIDs []int64) ([]Message, error)
	ListHistorySessionSummaries(ctx context.Context, userID, sessionID int64, limit int, cutoffMs int64, excludeIDs []int64) ([]HistorySessionSummary, error)

	InsertRun(ctx context.Context, run Run) (Run, error)
	GetRun(ctx context.Context, id int64) (*Run, error)
	LockRun(ctx context.Context, id int64) (*Run, error)
	ListWaitingRuns(ctx context.Context) ([]Run, error)
	HasDeletedRunHistory(ctx context.Context, run Run) (bool, error)
	GetRunByRequestID(ctx context.Context, userID int64, requestID string) (*Run, error)
	UpdateRun(ctx context.Context, run Run) error
	SetRunInput(ctx context.Context, runID int64, payload []byte, lastActivityMs int64) error
	RequestCancel(ctx context.Context, userID, runID int64) error
	RequestCancelAll(ctx context.Context, userID int64) error
	CancelOpenBackground(ctx context.Context, userID int64, sources []string) ([]Run, error)
	Claim(ctx context.Context, owner string, nowMs, leaseMs int64) (*Run, error)
	RenewLease(ctx context.Context, runID int64, owner string, generation, leaseUntilMs, heartbeatMs int64) (bool, error)
	OldestQueuedAgeMs(ctx context.Context, nowMs int64) (int64, error)
	AgentConsent(ctx context.Context, userID int64) (version int32, granted bool, err error)

	InsertEvent(ctx context.Context, runID int64, eventType string, payload []byte, createdAtMs int64) (Event, error)
	ListEventsAfter(ctx context.Context, runID, afterSeq int64) ([]Event, error)
	ListSourceEvents(ctx context.Context, runID int64) ([]Event, error)
	MaxEventSeq(ctx context.Context, runID int64) (int64, error)

	InsertToolCall(ctx context.Context, call ToolCall) (ToolCall, error)
	GetToolCall(ctx context.Context, runID int64, callID string) (*ToolCall, error)
	UpdateToolCall(ctx context.Context, call ToolCall) error
	ListToolCalls(ctx context.Context, runID int64) ([]ToolCall, error)

	GetJournal(ctx context.Context, userID int64, requestID, tool, digest string) (*Journal, error)
	ReserveJournal(ctx context.Context, row Journal) (*Journal, bool, error)
	CompleteJournal(ctx context.Context, id int64, status, resultJSON string) error
	ListSuccessfulJournal(ctx context.Context, userID int64, requestID string) ([]Journal, error)

	InsertSource(ctx context.Context, src Source) (Source, error)
	GetSources(ctx context.Context, runID int64, handles []string) ([]Source, error)
	ListSources(ctx context.Context, runID int64) ([]Source, error)
	PutEvidence(ctx context.Context, evidence Evidence) error
	ListEvidence(ctx context.Context, runID int64, handle string) ([]Evidence, error)
	SaveQuestion(ctx context.Context, question QuestionRequest) error
	ListQuestions(ctx context.Context, runID int64) ([]QuestionRequest, error)
	SavePresentation(ctx context.Context, presentation AnswerPresentation) error
	GetPresentation(ctx context.Context, messageID int64) (*AnswerPresentation, error)
	ClearResearchHistory(ctx context.Context, userID int64) error

	InsertConfirmation(ctx context.Context, row Confirmation) (Confirmation, error)
	GetConfirmation(ctx context.Context, runID int64, callID string) (*Confirmation, error)
	ResolveConfirmation(ctx context.Context, userID, runID int64, callID, digest string, approved bool, nowMs int64) (*Confirmation, error)
	GetInputCommand(ctx context.Context, userID int64, requestID string) (*InputCommand, error)
	InsertInputCommand(ctx context.Context, command InputCommand) (InputCommand, error)

	CountQueue(ctx context.Context, runID int64) (int, error)
	Enqueue(ctx context.Context, item QueueItem) (QueueItem, error)
	ListQueue(ctx context.Context, runID int64) ([]QueueItem, error)
	DeleteQueueThrough(ctx context.Context, runID, maxID int64) error
	DeleteQueue(ctx context.Context, runID int64) error

	InsertAlert(ctx context.Context, alert Alert) (bool, error)

	InsertOutbox(ctx context.Context, row Outbox) error
	ListUnpublishedOutbox(ctx context.Context, limit int) ([]Outbox, error)
	MarkOutboxPublished(ctx context.Context, ids []int64) error

	UpsertDeliveryBucket(ctx context.Context, userID, hitID, windowStartMs, nowMs int64) (DeliveryBucket, error)
	GetBucket(ctx context.Context, id int64) (*DeliveryBucket, error)
	GetPendingBucket(ctx context.Context, userID int64) (*DeliveryBucket, error)
	ListDueBuckets(ctx context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error)
	MarkBucketScheduled(ctx context.Context, id, runID int64) error
	MarkBucketSent(ctx context.Context, id, runID int64) error
	DeferBucket(ctx context.Context, id, notBeforeMs int64) error
	DismissBucket(ctx context.Context, id, runID int64) error
	ResetBucket(ctx context.Context, id, runID int64) error
	RequeueFailedBuckets(ctx context.Context) error
	ReserveWatchQuota(ctx context.Context, bucketID, userID int64, taskIDs []int64, dayStartMs, hourStartMs int64, dailyLimit, hourlyLimit int) (allowed bool, retryAtMs int64, err error)
	FinishWatchDelivery(ctx context.Context, id, userID, runID int64, delivered bool, nowMs int64) error
	ResetUnsentBuckets(ctx context.Context, userID int64) error
	CountSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) (int, error)
	IncrSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) error
}

type Notifier interface {
	Wake(ctx context.Context, runID int64) error
	WakeToken(ctx context.Context, runID int64) (string, error)
}
