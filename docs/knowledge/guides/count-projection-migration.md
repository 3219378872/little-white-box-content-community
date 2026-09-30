# Durable interaction-count projection rollout

This guide covers audit findings 3/4. It does not establish a production or real-MySQL validation result.

## Protocol

Interaction's existing relationship/count/outbox transaction increments the per-target
`action_count.revision` and embeds both count values in `count_snapshot`. Consumers
apply only a strictly newer target revision. One Content SQL transaction owns the
receipt, target lock/version, count replacement, and downstream post-count outbox.
Redis is never an event receipt. A zero-to-zero update is valid regardless of
`clientFoundRows`. Producer activation predicates also make active duplicates no-ops
under either affected-row mode. Comment projections update likes only; comments have
no favorite column. Older snapshots and redeliveries cannot increment/decrement counts.
Legacy and current model-cache keys are invalidated in separate single-key commands,
so Redis Cluster does not reject a cross-slot multi-key DEL.

`post.stats_seq` is a second, independent SQL sequence shared by interaction counts,
comment counts and lifecycle create/update snapshots. `pkg/poststats` advances it
under the same post lock/transaction as the mutation and outbox write. Lifecycle
payload counters are replaced with that locked snapshot, not preflight cached values.
A new post seeds the sequence from its actual create-event watermark; subsequent
allocation never uses wall time. Existing zero/uninitialized or exhausted sequences
fail closed and roll back instead of publishing a value Search would ignore.
Content `revision` continues to change only for content edits.

Search applies body revision and stats sequence independently in an atomic `_update`
script. Its internal `_version` is not a business watermark. Full lifecycle events
cannot overwrite newer count fields; count patches cannot suppress newer bodies.
Delete/unpublish retains a content-revision tombstone with no searchable body, and
Search/ES recall queries exclude that marker. A later higher content revision can
publish again; older full events cannot resurrect a tombstone. Exhausted ES update
conflicts retry rather than masquerade as already-applied business revisions.

## Existing-volume cutover (maintenance required)

1. Back up both authoritative databases. Apply the idempotent schema-only patch
   `deploy/sql/patches/20260930_count_projection.sql` and
   `deploy/sql/patches/20260930_post_stats_sequence.sql`.
2. Stop count-sync, Search consumers and **all Interaction and Content post/comment
   writers**; wait for transactions and in-flight indexing RPCs to finish. Deploy
   the new binaries but do not resume writes. Do not mix old/new writers or Search
   consumers after this point. Schema DDL alone leaves existing stats_seq at zero;
   new count/comment/edit transactions deliberately fail until seeded.
3. Determine a positive verified global watermark at least as large as every
   current SQL post.stats_seq, indexed Search stats_seq, and effective stats_seq in
   ALL retained/replayable lifecycle and count events (including outbox, broker,
   DLQ, archives and backups; legacy events without stats_seq use event_time).
   Do not estimate this from today's clock. If that bound cannot be established,
   cutover is blocked unless the unsupported old replay sources are explicitly
   retired under a separate approved plan. Search rebuild alone is not an escape
   from this requirement: a high legacy timestamp could otherwise outrank new data.
   In the same SQL session set `@legacy_stats_seq_floor` to that verified bound,
   then review/execute `deploy/sql/manual/20260930_count_projection_baseline.sql`
   on the shared authoritative MySQL. The unset/nonpositive/exhausted-floor guard
   rejects before data changes and also verifies the floor covers existing SQL
   sequences. For a genuinely empty installation with no historical events use 1.
   It recounts active relations, bumps target
   revisions, seeds Content counts/fences and stats sequences, then marks legacy reconciliation complete
   in the same transaction. It is deliberately not an automatic replay patch.
   Independently hosted databases require a coordinated snapshot/import procedure;
   do not run this cross-schema script against them.
4. Verify counts against active relation rows. Invalidate old Content model caches
   and rebuild Search with the SAME verified watermark:
   `make search-rebuild ARGS="-maintenance-confirmed -stats-seq-floor=<verified-watermark>"`.
   The rebuild command rejects an omitted watermark and stamps each reconstructed
   document with it, while SQL's next sequence is larger. Keep all writers paused
   through scan and alias promotion; this command has no online change capture.
   Preserve existing deletion tombstones or fence old lifecycle replay during any
   replacement-index migration; publishing a new collection does not magically
   restore historical tombstones. The SQL baseline itself emits no per-post search
   outbox events.
5. Resume only new writers and count-sync; verify lag and equality, including
   unlike/unfavorite-before-add, two users, restart, and lost ACK. Historical
   snapshotless events are receipt-only no-ops **only** after the baseline marker;
   before that they retry rather than silently losing counts.

Fresh databases need no recount for versioned events. They reject any legacy event
until an explicit baseline is completed. Existing accidental drift is not repaired
merely by applying schema DDL.

## Retention and rollback

Keep target version fences permanently. Keep event receipts at least 90 days and
longer than every enabled replay/backup horizon; no TTL/deletion job is added here.
A version fence still makes older snapshots harmless after receipt expiry. Keep all
new columns/tables on binary rollback. Pause writers/consumer before rollback;
returning to the old delta consumer requires restoring a coordinated count/offset
checkpoint and its legacy dedup state, or re-reconciling at another maintenance
window. A Search rollback must restore its matching index/checkpoint and consumer;
old external-version consumers are incompatible with new retained tombstones and
independent watermarks. Keep sequence columns and tombstones on rollback.
Never run an old writer after marking legacy reconciliation complete: its
snapshotless new events would be indistinguishable from pre-cutover historical data.

## Every later Search rebuild

The same maintenance and watermark rules apply on EVERY rebuild, even after the
first rollout. Pause Content post/comment writers, Interaction/count-sync and Search
consumers; drain in-flight writes. Determine a floor covering current SQL sequences,
Search documents and all still-supported legacy replay sources. Execute
`deploy/sql/manual/search_stats_watermark_seed.sql` with that verified
`@legacy_stats_seq_floor`, then rebuild with exactly the same value and
`-maintenance-confirmed`. The CLI rejects omitted maintenance confirmation or floor
before contacting services. A higher synthetic Search floor without first seeding
SQL would suppress legitimate future count updates, so it is prohibited. Keep the
pause through promotion and restart only compatible consumers afterward. Retain or
migrate deletion tombstones, or explicitly fence affected historical lifecycle
replay; fresh aliases alone do not carry their history.

## Verification

Targeted race tests exercise production `*sql.DB` code using a transactional driver:
rollback after receipt, before count/outbox/fence/commit, lost commit ACK, duplicate,
reordered, concurrent, zero-changed-row, legacy rejection, and comment paths.
Producer tests execute real `sqlstore` transaction/enqueue code with an injected
SQL driver. MySQL integration cases cover reverse order, concurrent snapshots,
restart and Redis absence. They must be executed against real MySQL before rollout;
compiling them is not runtime evidence. Broker kill/recovery, cross-schema baseline,
DSN affected-row modes, and the 30-second convergence SLO remain rollout gates.

The follow-up tests cover concurrent comment/like/lifecycle sequence allocation,
rollback, unseeded/overflow rejection, stale preflight payload replacement, identical
wall times and all six event orders. HTTP capture tests assert real adapter payloads
and emulate the comparison contract; they do not execute Painless. New MySQL and
Elasticsearch integration cases must run before deployment, including real Painless
compilation, aliases, migrations and live concurrent updates. Integration binaries
compiling is not evidence those service-level gates passed.
