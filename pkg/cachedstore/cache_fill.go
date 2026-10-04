package cachedstore

import (
	"context"
	"crypto/rand"
	"fmt"
)

// Reservations are not locks: concurrent readers can still query SQL, but only
// their owner may populate the cache. Expiry bounds crash/error residue, and a
// slow query whose reservation expires simply returns its SQL result uncached.
const fillReservationTTLSeconds = 30

const finishFillScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return 0
end
if ARGV[2] == '' or tonumber(ARGV[3]) <= 0 then
  return redis.call('DEL', KEYS[1])
end
redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
return 1
`

// newFillMarker returns a unique reservation marker, or "" when no randomness
// is available. It is intentionally not JSON, so it never decodes as a row.
func newFillMarker() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return ""
	}
	return fmt.Sprintf("fill:%x", token)
}

func (c CachedConn) reserveFill(ctx context.Context, key string) string {
	marker := newFillMarker()
	if marker == "" {
		return ""
	}
	acquired, err := c.redis.SetnxExCtx(ctx, key, marker, fillReservationTTLSeconds)
	if err != nil || !acquired {
		return ""
	}
	return marker
}
