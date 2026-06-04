# Message Service Design

## Goal
Implement Phase 2 W3 message service for notifications, unread counts, and basic private-message RPCs using go-zero conventions.

## Architecture
Add `app/message` as a go-zero zrpc service generated from the existing `proto/message/message.proto`. The service owns `conversation`, `message`, and `notification` data from `deploy/sql/xbh_message.sql`, with custom model methods for pagination, unread counts, read marking, and conversation upsert.

## Components
- `app/message`: RPC entrypoint, generated server/client, logic, models, config, and MQ consumers.
- `internal/svc`: dependency injection for MySQL, Redis, and model interfaces.
- `internal/logic`: RPC behavior; handlers validate inputs, use context-aware model calls, return `errx.New` business errors.
- `internal/mqs`: RocketMQ consumer converting user-action events into notifications.

## Data Flow
`SendNotification` and the MQ consumer insert notification rows and invalidate/increment unread cache. `GetUnreadCount` reads Redis first, falls back to MySQL counts, then caches the result. `MarkRead` updates message and notification read state for a user/conversation and invalidates unread cache.

## Testing
Use TDD for custom logic and consumer code. Unit tests use fake model interfaces and Redis-store abstractions where possible; generated code is not tested directly.
