# Message Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Phase 2 W3 Message RPC service with notifications, unread counts, and private-message basics.

**Architecture:** Generate a go-zero zrpc service from `proto/message/message.proto`, generate/cache models from `deploy/sql/xbh_message.sql`, then add custom model methods and logic implementations behind interfaces for focused tests. Redis caches unread counts; MQ consumer creates notifications from user-action events.

**Tech Stack:** Go 1.26.1, go-zero zrpc/sqlx/cache/redis, RocketMQ wrapper `pkg/mqx`, MySQL schema in `deploy/sql/xbh_message.sql`, `pkg/errx` business errors.

---

### Task 1: Service Skeleton

**Files:**
- Create/modify generated files under `app/message/`
- Modify: `go.work`

- [ ] Generate RPC skeleton from `proto/message/message.proto` using `goctl rpc protoc proto/message/message.proto --go_out=app/message --go-grpc_out=app/message --zrpc_out=app/message --style go_zero`.
- [ ] Generate models from `deploy/sql/xbh_message.sql` into `app/message/internal/model` with cache support where compatible.
- [ ] Add `./app/message` to `go.work`.
- [ ] Verify `go test ./app/message/...` reaches compile stage.

### Task 2: Service Context and Model Interfaces

**Files:**
- Modify: `app/message/internal/config/config.go`
- Modify: `app/message/internal/svc/service_context.go`
- Modify/Create: `app/message/internal/svc/redis_store.go`

- [ ] Define config fields for `DataSource`, `Cache`, `Redis`, and `MQ`.
- [ ] Define narrow model interfaces used by logic tests.
- [ ] Instantiate generated models and Redis in `NewServiceContext`.
- [ ] Add Redis store helpers for get/set/delete unread cache.

### Task 3: Notification Logic

**Files:**
- Modify: `app/message/internal/logic/send_notification_logic.go`
- Modify: `app/message/internal/logic/get_notifications_logic.go`
- Create tests in `app/message/internal/logic/*_test.go`

- [ ] RED: tests for invalid notification request, successful insert, filtered pagination.
- [ ] GREEN: implement validation, insert, conversion to protobuf, and pagination.
- [ ] REFACTOR: keep conversion helpers small and context-safe.

### Task 4: Unread and Read Logic

**Files:**
- Modify: `app/message/internal/logic/get_unread_count_logic.go`
- Modify: `app/message/internal/logic/mark_read_logic.go`
- Create tests in `app/message/internal/logic/*_test.go`

- [ ] RED: tests for Redis hit, DB fallback, cache fill, and mark-read invalidation.
- [ ] GREEN: implement unread counting and read marking.
- [ ] REFACTOR: centralize unread cache key naming.

### Task 5: Private Message Logic

**Files:**
- Modify: `app/message/internal/logic/send_message_logic.go`
- Modify: `app/message/internal/logic/get_conversations_logic.go`
- Modify: `app/message/internal/logic/get_messages_logic.go`
- Create tests in `app/message/internal/logic/*_test.go`

- [ ] RED: tests for send message validation, conversation list, cursor message list.
- [ ] GREEN: implement conversation upsert for both users, message insert, and list conversion.
- [ ] REFACTOR: keep SQL details in model methods.

### Task 6: MQ Notification Consumer

**Files:**
- Create: `app/message/internal/mqs/message_consumer.go`
- Create: `app/message/internal/mqs/message_consumer_test.go`
- Modify: `pkg/mqx/topics.go` if a message consumer group constant is missing.

- [ ] RED: tests for like/comment/follow/system event templates and malformed payload retry behavior.
- [ ] GREEN: implement JSON event parsing, notification insert, and unread cache invalidation.
- [ ] REFACTOR: isolate template rendering.

### Task 7: Verification

**Files:**
- All changed files

- [ ] Run `go test ./app/message/...`.
- [ ] Run `go test ./pkg/mqx/...` if topic constants changed.
- [ ] Run `go test ./...` if focused tests pass and workspace modules resolve.
