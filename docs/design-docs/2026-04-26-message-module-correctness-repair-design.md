# Message Module Correctness Repair Design

## Goal

Fix the message module correctness risks found in review while keeping this round scoped to HIGH issues plus the core MEDIUM issues selected by the user.

## Approved Scope

This repair includes:

- Keep conversation unread counts consistent with message read state.
- Make private-message creation transactional across conversation upserts and message insert.
- Prevent malformed or unsupported MQ events from retrying forever.
- Preserve real system errors when conversation ownership checks hit database failures.
- Add the message-query indexes needed by the current read path.
- Add bounded validation for message type, notification type, title length, and content length.

This repair excludes:

- Full 80% coverage cleanup across `app/message`.
- New Redis or RocketMQ integration-test coverage.
- Gateway aggregation for `target_user_name` and `target_user_avatar`.
- Breaking proto redesign or broad conversation data-model migration.

## Current Context

`app/message` is a go-zero zrpc service generated from `proto/message/message.proto`. Business code lives in `internal/logic`, custom SQL lives in non-generated model files under `internal/model`, dependencies are exposed through narrow interfaces in `internal/svc`, and RocketMQ event handling lives in `internal/mqs`.

The current implementation already validates basic user IDs, checks conversation ownership for `GetMessages` and conversation-scoped `MarkRead`, separates message-read behavior from notification-read behavior, and has focused unit tests for logic and MQ rendering.

The remaining correctness problems are mostly cross-table consistency and failure classification:

- `conversation.unread_count` increments when a message is sent but is not decremented when messages are marked read.
- Message creation updates conversation rows before inserting the message, without a transaction.
- MQ permanent payload errors and transient database errors are both treated as retryable.
- Conversation ownership lookup errors are all mapped to permission denial.
- Message history queries do not have a matching composite index.
- Enum and length validation is too loose for the current DDL.

## Architecture

Add a small write-side command model under `app/message/internal/model` to own cross-table mutations. Logic methods will continue to validate requests, check ownership where needed, invoke model interfaces with the request context, invalidate unread cache, and return `errx` business errors.

The command model will use `sqlx.SqlConn.TransactCtx` for message creation because the operation spans two `conversation` rows and one `message` row. It will use atomic SQL updates for read marking and unread-count adjustment, so concurrent sends and reads do not require a read-modify-write cycle in logic.

Keep generated files untouched. Changes stay in non-generated model files, logic files, `internal/svc/service_context.go`, MQ consumer code, tests, and `deploy/sql/xbh_message.sql`.

## Components

### `MessageCommandModel`

Create `app/message/internal/model/message_command_model.go` with an interface and concrete implementation:

- `CreateMessageWithConversations(ctx, senderID, receiverID int64, content string, msgType int64) (int64, error)`
- `MarkConversationRead(ctx, userID, targetUserID int64) (int64, error)`

`CreateMessageWithConversations` performs these operations inside one transaction:

1. Upsert the sender conversation with `unread_count + 0`.
2. Upsert the receiver conversation with `unread_count + 1`.
3. Read the receiver conversation ID by `(receiver_id, sender_id)`.
4. Insert the message with `conversation_id` set to the receiver conversation ID.
5. Return the inserted message ID.

`MarkConversationRead` performs these operations safely:

1. Mark unread messages from `targetUserID` to `userID` as read and capture `RowsAffected`.
2. If affected rows are greater than zero, decrement the current user's conversation unread count by that amount using `greatest(unread_count - ?, 0)`.
3. Return the affected message count.

### Service Context

Add a `MessageCommandModel` dependency to `app/message/internal/svc/service_context.go`. The logic tests can keep using fakes through a narrow interface instead of constructing real DB dependencies.

### Logic

`SendMessageLogic` will validate `msg_type` as one of the supported message types and validate content length before calling `MessageCommandModel.CreateMessageWithConversations`. It no longer coordinates separate conversation and message writes itself.

`MarkReadLogic` will keep the current behavior split:

- With `conversation_id`: verify the conversation belongs to `user_id`, call `MessageCommandModel.MarkConversationRead`, then invalidate unread cache.
- Without `conversation_id`: mark all notifications read, then invalidate unread cache.

`GetMessagesLogic` and `MarkReadLogic` will distinguish `model.ErrNotFound` from other lookup errors. Not found or not owned returns permission denial; database and infrastructure failures return `SystemError`.

`SendNotificationLogic` will validate notification type and title/content lengths before insert.

### MQ Consumer

Introduce a small permanent-error marker for payload errors:

- malformed JSON
- missing target user
- missing or unsupported action type
- unsupported notification template
- system notification with empty content

`MessageConsumer.Consume` can return this marker for permanent errors. The RocketMQ handler will acknowledge permanent errors after logging them with context and will retry transient insert errors.

### SQL and Indexing

Update `deploy/sql/xbh_message.sql` with composite indexes that match the current access paths:

- message history: `(sender_id, receiver_id, id)` and `(receiver_id, sender_id, id)`
- unread count and mark-read path: `(receiver_id, status, sender_id)`
- notification list/count path: `(user_id, type, id)` and `(user_id, status)`

No model regeneration is needed because the table fields do not change.

## Data Flow

### Send Message

RPC request enters `SendMessageLogic`. The logic trims and validates content, validates `msg_type`, calls the command model with the current `ctx`, and invalidates unread cache for the receiver after a successful transaction.

If any DB statement fails, the transaction rolls back. The response is returned only after the message row exists.

### Mark Conversation Read

RPC request enters `MarkReadLogic`. The logic validates `user_id`, validates ownership of `conversation_id`, and passes the caller plus conversation target to the command model. The command model marks only messages sent by the target user to the caller as read, then decrements the caller's conversation unread count by the number of rows that were actually changed.

### Consume Notification Event

RocketMQ delivers a user-action event. Malformed or unsupported events are logged and acknowledged as permanent failures. Valid events insert one notification row and invalidate the target user's unread cache. Transient insert errors still return retry to RocketMQ.

## Error Handling

- Validation failures return `errx.ParamError`.
- Conversation not found or not owned returns `errx.PermissionDenied`.
- Database errors return `errx.SystemError`.
- Permanent MQ payload errors are logged and skipped instead of retried.
- Transient MQ persistence errors are logged and retried.

All DB, Redis, and MQ calls continue to use the request or consumer `ctx`. Logs must use context-aware loggers.

## Testing

Use TDD for each repair:

- Add logic tests proving `SendMessageLogic` calls the transactional command model and rejects invalid message types or oversized content.
- Add logic tests proving `MarkReadLogic` calls the command model for conversation reads, does not mark notifications in that path, and maps not-found vs DB errors differently.
- Add logic tests for notification type and length validation.
- Add MQ tests proving permanent payload/template errors do not request retry, while DB insert errors do.
- Add targeted MySQL testcontainers model tests for `MessageCommandModel` transaction behavior. These tests should cover successful message creation, rollback when message insert fails, and conversation unread decrement on mark-read.

Verification for this scoped repair:

- `GOCACHE=/tmp/go-build-cache go test ./app/message/...`
- `GOCACHE=/tmp/go-build-cache go test -race ./app/message/internal/logic ./app/message/internal/mqs`
- `GOCACHE=/tmp/go-build-cache go vet ./app/message/...`
- `GOCACHE=/tmp/go-build-cache golangci-lint run ./app/message/...`

Full-repo `go test ./... -race -cover`, `go vet ./...`, and `golangci-lint run` remain broader project checks and may still surface unrelated repository debt. This repair should not hide or worsen unrelated failures.
