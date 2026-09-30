# Immutable versioned embedding projections

Audit finding 13 is a projection consistency issue. Existing Content visibility
checks remain in place; this change does not claim an information-disclosure fix.

## Durable boundary

Milvus has no compare-and-swap on a scalar revision. A read-before-upsert, process
mutex, Redis lock, or SQL lock around a remote RPC cannot fence an old RPC that
arrives after its caller crashes. The new physical schema uses immutable identity
`projection_id = post_id:revision`, plus `post_id`, `revision`, and `deleted`.
Deletion adds `post_id:revision:deleted`; it never destroys the revision fence.
A same-revision tombstone wins over any live row. Retries use stable keys.

The effective projection is the maximum persisted revision, with tombstones winning
ties. Every production vector reader resolves that view using strong-consistency
queries. Seed vectors use only the latest live row. ANN hits carry their post ID and
revision; old or tombstoned hits are excluded after a strong latest-version query.
A delayed remote write can add an obsolete physical row but cannot overwrite or
resurrect the effective projection. This is an append-and-resolve protocol, not an
unsupported claim that Milvus provides atomic conditional upsert.

Embedding consumption also reconciles the current Content authority before writing.
It embeds current published content/revision rather than an old event body; confirmed
absence writes a tombstone. RPC failure or a nil response retries. An empty successful
protobuf repeated field is confirmed absence. This authority check prevents a newly processed historical event from indexing a deleted
post after a coordinated fresh rebuild. Absence does not expose the hidden post's
actual revision: its tombstone uses at least the event/stored revision. Therefore it
is NOT a fence against an already in-flight old RPC crossing alias promotion.
Runtime writers additionally pin the alias to the canonical physical schema name at
startup; delayed RPCs from that process remain on the old physical collection after
promotion. ContentRpc is required at runtime.

## Rollout and rebuild

1. Stop Content mutations, drain all lifecycle outbox messages and embedding work,
   then stop embedding consumers. Preserve the old physical collection and alias.
   Confirm there are no unresolved mutation RPCs from pre-pinning writers. If an
   old writer timed out or crashed with unknown remote status, promotion is blocked
   until that ambiguity is resolved or its access to the target is safely fenced.
   Merely killing its local process or waiting a guessed interval is insufficient.
   Do not run an uncoordinated online rebuild: the existing rebuild command does
   not dual-write or capture updates between its scan and alias promotion.
2. Build a NEW physical collection with the new binary and schema; never alter the
   old collection in place. Rebuild now carries `PostInfo.revision` into every row.
   The schema validator rejects old collections rather than silently discarding
   versions. Verify row count, model/version/dimension and representative searches.
3. Promote and verify the alias, then start new recall readers and restart all
   consumers so they pin the new physical target. Never start a consumer before
   promotion and assume its write destination follows later alias changes. The new
   reader intentionally fails closed on old-schema metadata. Use a maintenance
   window or a separately named new alias while staging the new deployment.
4. Resume Content writers and verify event lag, stale update/deletion tests and
   authoritative filtering. Replay historical messages only through the new
   authority-validating consumer. Never send old writer binaries to the new alias.

Unreferenced historical collections should only be removed under an approved
retention plan after rollback is no longer needed. Tombstones/live histories in the
active collection are not TTL'd. Latest-version queries are bounded at 16,384 rows
per requested ID set and fail closed at that cap, rather than accept a truncated
history. Monitor history growth and ANN yield. Old versions can occupy ANN slots;
periodic coordinated rebuilds compact them. This can reduce recall breadth before a
rebuild, but stale versions cannot be returned as current. Existing recommend
fallbacks and Content authority filtering still apply.

## Rollback

Pause writers and consumers before swapping binaries/aliases. Roll back the readers,
consumer and physical collection together, retaining both collections. The old
protocol is not replay-safe: reconcile from Content and plan a fresh forward rebuild
before resuming historical replay. Do not erase new tombstones or down-convert the
new collection to the old schema. A rollback that allows old mutation logic to run
restores the original defect and is an operational recovery, not equivalent safety.

## Verification boundaries

Race tests exercise actual Milvus adapter schema/column construction, immutable
remote-state capture, lost ACK/restart, delayed old writes, and delete/update ties.
Consumer tests cover current authority, absence after rebuild, unavailable authority,
and a lagging published revision. Reader tests exclude stale/deleted ANN hits and
reject incomplete history/old schema. Integration tests compile for real Milvus
reconnect/concurrency and real recall, but require execution with actual Milvus
before rollout. No live Milvus, broker crash test, model service, production rollout,
or convergence/recall benchmark was run in the repair sandbox.
