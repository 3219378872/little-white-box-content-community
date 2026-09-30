# Disposable model cache

A miss reserves the same Redis key with a unique, non-JSON marker before reading
SQL. Successful and not-found results replace that marker only through an atomic
single-key Lua compare-and-fill. Post-commit invalidation deletes both cached
values and reservations, so an older read cannot refill the successfully
invalidated key. Concurrent readers that do not own the marker read SQL without
filling the cache. Reservations expire after 30 seconds; expiry, eviction, or a
failed/unconfirmed reservation prevents that read from filling. An abandoned
reservation only reduces cache effectiveness until it expires.

Positive and negative entries keep their configured TTLs (defaults: seven days
and 60 seconds). Non-positive TTLs disable filling. Failed SQL or JSON encoding
releases the marker only if it is still owned. Release/fill failures do not
replace the SQL result. Each fill and invalidation uses one key, including when
multiple invalidations are requested, so no cross-slot Lua or DEL is required.

## Consistency boundary

This is a disposable cache, not an authorization authority. An invalidation
failure can leave an existing value or reservation live; Redis outages, lost
writes or restoration of old Redis state are not solved by fill fencing. A
committed SQL write still succeeds when invalidation fails. Security-sensitive
status/ownership reads must bypass it; `MediaModel.FindOne` and batch media
lookups read the authoritative SQL store directly. A read already in progress
may return the SQL snapshot it actually observed; the guarantee concerns later
cache fills and reads after successful invalidation.

Index values contain only the primary ID. The primary lookup always performs a
fresh read on a primary-cache miss; it cannot reuse a row from an earlier index
query whose snapshot may precede primary invalidation.

## v2 to v3 rollout

The `cache:v3:` namespace excludes values filled by the previous unfenced
implementation. This change is not safe for a mixed-version rolling deployment:
v2 writers do not invalidate v3 entries, and v3 writers do not invalidate v2
entries. Drain/stop the old application and background writers/readers, and
prevent mixed-version writes while deploying the complete set of cache users.
After the old writers are drained, clear both v2 and v3 disposable namespaces
before resuming traffic on the new version; clearing v3 also removes entries
created during any overlap. Namespace cleanup is an operator rollout step, not
an automatic application startup deletion. A rollback requires the same
coordination and cleanup. Old v2 entries may otherwise remain in Redis until
their existing TTL expires, but v3 readers never consult them.

## Verification

The ordinary tests execute the real Redis client over a deterministic test-local
RESP server and use controlled SQL callbacks. Media logic regressions execute
the real model and sqlstore with an injected database/sql driver. These are unit
and protocol tests, not live Redis/MySQL or object-download evidence.

`go test -tags=integration ./pkg/cachedstore` adds real Redis/MySQL tests through
the repository's isolated test environment, including update/delete/create
interleavings, configured TTLs and the actual Lua expiry/ownership comparison.
It requires the integration dependencies; merely compiling this suite does not
establish a live integration pass.
