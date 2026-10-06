package cachedstore

import (
	"context"
	"encoding/json"

	"esx/pkg/sqlstore"
)

// QueryRowsByIDs reads one cached row per ID and loads every miss with one call
// to load. Each key keeps the single-key fencing of QueryRowCtx: only a reader
// that reserved an empty key before the SQL read may fill it, and a successful
// invalidation removes that reservation. IDs absent from load's result are
// cached as not found. Rows are returned keyed by ID; missing IDs are omitted.
//
// Redis failures never replace the SQL result: a failed batch GET loads every
// ID from SQL without filling, and failed reservations or fills only reduce
// cache effectiveness.
func QueryRowsByIDs[K comparable, T any](
	ctx context.Context,
	c CachedConn,
	ids []K,
	key func(K) string,
	load func(context.Context, sqlstore.SqlConn, []K) (map[K]T, error),
) (map[K]T, error) {
	rows := make(map[K]T, len(ids))
	if len(ids) == 0 {
		return rows, nil
	}
	if c.redis == nil {
		return load(ctx, c.conn, ids)
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = Prefix + key(id)
	}
	values, err := c.redis.GetManyCtx(ctx, keys)
	if err != nil {
		return load(ctx, c.conn, ids)
	}
	missing := make([]K, 0, len(ids))
	var reserveIDs []K
	var reserveKeys []string
	for i, id := range ids {
		if values[i] == "" {
			missing = append(missing, id)
			reserveIDs = append(reserveIDs, id)
			reserveKeys = append(reserveKeys, keys[i])
			continue
		}
		var cached struct {
			Missing bool
			Value   json.RawMessage
		}
		if json.Unmarshal([]byte(values[i]), &cached) == nil {
			if cached.Missing {
				continue
			}
			var row T
			if len(cached.Value) > 0 && json.Unmarshal(cached.Value, &row) == nil {
				rows[id] = row
				continue
			}
		}
		// Another reader's reservation or an unreadable value: read SQL
		// without owning the key, exactly like QueryRowCtx.
		missing = append(missing, id)
	}
	if len(missing) == 0 {
		return rows, nil
	}
	markers := reserveFills(ctx, c, reserveKeys)
	loaded, err := load(ctx, c.conn, missing)
	if err != nil {
		releaseFills(ctx, c, reserveKeys, markers)
		return nil, err
	}
	finishFills(ctx, c, reserveIDs, reserveKeys, markers, loaded)
	for _, id := range missing {
		if row, ok := loaded[id]; ok {
			rows[id] = row
		}
	}
	return rows, nil
}

// reserveFills acquires one reservation per empty key. markers[i] is empty when
// key i was not reserved.
func reserveFills(ctx context.Context, c CachedConn, keys []string) []string {
	markers := make([]string, len(keys))
	if len(keys) == 0 {
		return markers
	}
	candidates := make([]string, len(keys))
	for i := range keys {
		candidates[i] = newFillMarker()
	}
	acquired, err := c.redis.SetnxExManyCtx(ctx, keys, candidates, fillReservationTTLSeconds)
	if err != nil {
		// An unconfirmed reservation cannot fill; leftovers expire.
		return markers
	}
	for i, ok := range acquired {
		if ok && candidates[i] != "" {
			markers[i] = candidates[i]
		}
	}
	return markers
}

// releaseFills gives back reservations without writing a value, e.g. when the batch query failed.
func releaseFills(ctx context.Context, c CachedConn, keys, markers []string) {
	var ownedKeys []string
	var args [][]any
	for i, marker := range markers {
		if marker == "" {
			continue
		}
		ownedKeys = append(ownedKeys, keys[i])
		args = append(args, []any{marker, "", 0})
	}
	if len(ownedKeys) > 0 {
		_ = c.redis.EvalEachCtx(ctx, finishFillScript, ownedKeys, args)
	}
}

// finishFills writes each loaded row (or a not-found marker) into the keys this batch reserved;
// keys reserved by someone else or invalidated meanwhile are left untouched by the script.
func finishFills[K comparable, T any](ctx context.Context, c CachedConn, ids []K, keys, markers []string, loaded map[K]T) {
	var ownedKeys []string
	var args [][]any
	for i, marker := range markers {
		if marker == "" {
			continue
		}
		row, found := loaded[ids[i]]
		cached := struct {
			Missing bool
			Value   any
		}{Missing: !found}
		ttl := c.options.NotFoundTTLSeconds
		if found {
			cached.Value = row
			ttl = c.options.TTLSeconds
		}
		value := ""
		if b, err := json.Marshal(cached); err == nil {
			value = string(b)
		}
		ownedKeys = append(ownedKeys, keys[i])
		args = append(args, []any{marker, value, ttl})
	}
	if len(ownedKeys) > 0 {
		// Only each original owner can fill or release its key.
		_ = c.redis.EvalEachCtx(ctx, finishFillScript, ownedKeys, args)
	}
}
