# W4 DTM Distributed Transactions Design

Date: 2026-04-26

## Scope

Implement W4 DTM support for the cross-service transaction path from `doc/phases/phase-2-interaction.md`:

- Post creation plus Feed fanout.

Search indexing is explicitly out of scope for this W4 slice. The Search RPC branch from the phase document will be handled with Phase 3 search work, not as a placeholder or empty branch here.

DTM transaction state must use MySQL. Redis is not acceptable as the DTM storage backend.

Like action plus like-count update is explicitly out of DTM scope. Both `like_record` and `action_count` are owned by the interaction service and stored in the interaction MySQL database, so this path should use local database consistency patterns rather than a distributed transaction coordinator.

## Current State

The middleware compose file already has a `dtm` service, but it stores transaction state in Redis. That must be replaced with MySQL-backed storage and a durable `dtm` database.

`CreatePost` currently writes the post, writes tags, then directly publishes a RocketMQ `post-create` event. This has a DB/MQ consistency gap and does not use DTM.

`Like` currently upserts `like_record` and then increments `action_count`. Count update failures are logged and swallowed, which allows visible inconsistency between the action record and counter. This is an interaction-local consistency issue; it should be handled with a local MySQL transaction or an explicitly accepted eventually-consistent counter repair flow, not with DTM Saga.

Feed already has `feed_outbox`, `feed_inbox`, and a `PushToInbox` RPC. It does not yet expose a DTM branch endpoint that performs the full fanout step.

## Architecture

Use DTM gRPC APIs behind small local interfaces so logic tests can assert transaction intent without requiring a live DTM server.

Add DTM configuration to services that start or receive transaction branches:

- `content`: `DtmServer`, `ContentBusiServer`, `FeedBusiServer`.
- `feed`: `FeedBusiServer` if the branch URL cannot be derived safely from existing config.

Branch URLs must be config-driven and must not be hard-coded. The config value should point to the gRPC business address that DTM can call from the DTM container or deployment network.

Use the DTM SDK package `github.com/dtm-labs/dtm/client/dtmgrpc` unless implementation verification shows this repository needs the smaller standalone package. The design depends on documented reliable message capabilities for the content-to-feed flow. Do not introduce Saga for interaction likes in this W4 slice.

## Deployment

Change `deploy/docker-compose.middleware.yml` so the DTM service uses MySQL:

- `STORE_DRIVER: mysql`
- `STORE_DSN: "${DTM_STORE_DSN}"`
- `depends_on` should include healthy MySQL, not Redis.

Add durable initialization for the DTM store database:

- Add `CREATE DATABASE IF NOT EXISTS dtm ...` to the SQL initialization area or an equivalent `deploy/sql/` file.
- Use an environment placeholder for `DTM_STORE_DSN`, for example `root:${MYSQL_ROOT_PASSWORD}@tcp(mysql:3306)/dtm?parseTime=true`.

The middleware service may still depend on Redis for other application features, but DTM storage must not use Redis.

## Post Creation Transaction

`CreatePost` remains the public content RPC method.

The transaction flow is:

1. Validate request and generate the post ID before starting DTM work.
2. Create a DTM two-phase message transaction.
3. Add one branch: Feed fanout for the created post.
4. Execute the local post and tag writes through the DTM local transaction callback.
5. Submit the DTM message so Feed fanout is invoked only after the local content transaction is committed.

Because the existing `PostModel.InsertPost` and `PostTagModel.BatchInsertTagsByPostId` use `sqlx.SqlConn`, implementation should add model methods that can write through a transaction/session or a `*sql.DB` path compatible with `DoAndSubmitDB`. The local transaction must include both the post row and tag rows. It must not keep the old pattern where post insert and tag insert are separate transactions.

Feed receives a new DTM branch RPC:

- `FanoutPost(FanoutPostReq) returns (FanoutPostResp)`

`FanoutPost` does the same durable work as the current post-created consumer path:

- Write `feed_outbox` idempotently.
- If the author is below `BigVThreshold`, read followers and batch insert `feed_inbox` idempotently.

The current RocketMQ consumer can remain for existing asynchronous flows, but `CreatePost` should not publish the same feed event after the DTM branch succeeds. Otherwise Feed may receive duplicate work from two systems. Idempotent insert keys protect the data, but the code should avoid intentional double dispatch.

## Interaction Like Consistency

`Like` remains the public interaction RPC method.

The public method validates input and keeps all writes inside the interaction service boundary. Because the like record and counter live in the same service and database, the strict-consistency implementation should use `sqlx.SqlConn.TransactCtx`:

1. Upsert `like_record` from missing/inactive to active.
2. Detect duplicate active likes and return `errx.AlreadyLiked` without changing the counter.
3. Increment `action_count.like_count` only when the state actually changed to active.
4. Commit both writes together, or roll both back on failure.

If product requirements accept eventually consistent counters, the current "record succeeds, counter can be repaired" pattern may remain, but the document must name that trade-off explicitly. It must not be represented as a distributed transaction requirement.

Do not add interaction Saga branch RPCs such as `LikeAction`, `LikeActionRevert`, `IncrLikeCount`, or `DecrLikeCount`. Do not add an interaction DTM marker table for likes.

## Idempotency And Compensation

All branch methods must tolerate retry. The minimum guarantees are:

- Feed fanout uses existing unique keys on `feed_outbox` and `feed_inbox`.

If DTM barrier support is straightforward with gRPC branch context in this repository, branch methods should use DTM branch barriers for stronger retry and compensation idempotency. If not, implementation must document the SQL-level idempotency used for each branch and cover it with tests.

Compensation errors must be returned to DTM, not swallowed. Application logs should use `logx.WithContext(ctx)` through the logic logger.

## Error Handling

Logic methods keep returning `errx.NewWithCode(...)` for application errors. DTM submission failures map to `errx.SystemError`.

Public `Like` must not depend on DTM. If strict counter consistency is chosen, count update failures should cause the local transaction to roll back and the public call to fail with `errx.SystemError`. If eventually consistent counters are chosen, count update failures must be observable and repairable.

Branch methods should return success for idempotent retry states and real errors for storage failures. They must not use bare string errors in logic-level paths.

## Testing

Implementation must follow TDD.

Unit tests:

- DTM compose/config uses MySQL storage, not Redis.
- `CreatePost` builds a DTM message with the Feed fanout branch.
- `CreatePost` local transaction writes both post and tags and does not publish the legacy feed MQ event in the DTM path.
- `FanoutPost` is idempotent for repeated branch calls.
- `Like` does not require DTM config or interaction Saga branch RPCs.
- If strict counter consistency is implemented, `Like` rolls back the like state when `action_count` increment fails.

Integration tests:

- MySQL-backed feed branch flow: fanout writes outbox and inbox rows once across retries.
- Optional MySQL-backed interaction flow if like consistency is changed: first like creates active record and count 1; repeated like returns already-liked; simulated count failure leaves no active like state change.

Verification before completion:

- Regenerate changed proto output with goctl.
- `GOCACHE=/tmp/go-build go test ./app/interaction/... ./app/feed/... ./app/content/...`
- Broader repository checks should follow the project completion checklist when implementation is done.

## Non-Goals

- Do not implement Search RPC or Search indexing branches in this W4 slice.
- Do not keep Redis as DTM storage.
- Do not implement DTM Saga for interaction likes.
- Do not hand-edit generated goctl files.
- Do not introduce unrelated MQ consumers or recommendation behavior.
