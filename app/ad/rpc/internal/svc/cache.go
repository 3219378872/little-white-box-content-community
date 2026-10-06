package svc

import (
	"sync"
	"time"
)

// Cache 是投放路径的短期进程内缓存。条目最长保留 ttl（≤30 秒），
// 与 ad-mq 立即移出索引合计，暂停与下线在 60 秒内对新请求生效（ADS-032）。
type Cache struct {
	ttl  time.Duration
	mu   sync.Mutex
	data map[string]cacheEntry
}

// cacheEntry 是缓存值及其过期时间。
type cacheEntry struct {
	value   any
	expires time.Time
}

// NewCache 创建指定 TTL 的缓存。
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, data: map[string]cacheEntry{}}
}

// Get 返回未过期的值。
func (c *Cache) Get(key string, now time.Time) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data[key]
	if !ok || now.After(entry.expires) {
		delete(c.data, key)
		return nil, false
	}
	return entry.value, true
}

// Put 写入值；超过容量时整体清空，避免无界增长。
func (c *Cache) Put(key string, value any, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) > 4096 {
		c.data = map[string]cacheEntry{}
	}
	c.data[key] = cacheEntry{value: value, expires: now.Add(c.ttl)}
}
