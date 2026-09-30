package store

import (
	"context"
	"fmt"
)

// UpgradeDedupRetention requires a standalone Redis node; consumer configuration
// rejects Cluster because a single SCAN cannot enumerate every master.
// It repairs live pre-upgrade receipts without resetting
// their original processing age. Legacy keys that have already expired cannot
// be reconstructed here and require an authoritative replay/rebuild plan.
func (s *RedisBehaviorStore) UpgradeDedupRetention(ctx context.Context, legacyTTLSeconds int) (int, error) {
	if s.dedupTTLSeconds != DefaultBehaviorDedupTTL || legacyTTLSeconds <= 0 || legacyTTLSeconds > s.dedupTTLSeconds {
		return 0, fmt.Errorf("invalid legacy dedup TTL")
	}
	scanner, ok := s.redis.(redisKeyScanner)
	if !ok {
		return 0, fmt.Errorf("dedup retention upgrade requires bounded Redis SCAN")
	}
	upgraded := 0
	for _, pattern := range []string{
		"feature:" + s.featureVersion + ":dedup:*",
		"feature:" + s.featureVersion + ":u:*:exposure:dedup:*",
	} {
		var cursor uint64
		for {
			if err := ctx.Err(); err != nil {
				return upgraded, err
			}
			keys, next, err := scanner.ScanCtx(ctx, cursor, pattern, 256)
			if err != nil {
				return upgraded, fmt.Errorf("scan legacy dedup receipts: %w", err)
			}
			for _, key := range keys {
				changed, err := s.redis.EvalCtx(ctx, upgradeDedupRetentionScript, []string{key}, int64(legacyTTLSeconds)*1000, int64(s.dedupTTLSeconds)*1000)
				if err != nil {
					return upgraded, fmt.Errorf("upgrade legacy dedup receipt: %w", err)
				}
				if count, ok := changed.(int64); ok {
					upgraded += int(count)
				}
			}
			if next == 0 {
				break
			}
			cursor = next
		}
	}
	return upgraded, nil
}

const upgradeDedupRetentionScript = `
local marker = redis.call('GET', KEYS[1])
if not marker or marker == 'v3' then return 0 end
if marker ~= '1' then return redis.error_reply('unknown dedup receipt format') end
local remaining = redis.call('PTTL', KEYS[1])
if remaining == -2 then return 0 end
if remaining < 0 then return redis.error_reply('legacy dedup receipt has no expiry') end
local legacy = tonumber(ARGV[1])
local desired = tonumber(ARGV[2])
-- A longer-lived explicit-exclusion receipt already uses the required budget.
-- Ordinary legacy receipts preserve age: remaining + (90 days - old TTL).
if remaining <= legacy then
  remaining = remaining + desired - legacy
end
remaining = math.min(remaining, desired)
if remaining <= 0 then return 0 end
redis.call('SET', KEYS[1], 'v3', 'PX', remaining)
return 1
`
