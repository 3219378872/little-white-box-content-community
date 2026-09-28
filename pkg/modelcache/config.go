// Package modelcache defines the application's disposable model cache.
package modelcache

import "esx/pkg/redisstore"

type NodeConf struct {
	redisstore.RedisConf
	Weight int
}
type CacheConf []NodeConf
type Option func(*Options)
type Options struct {
	TTLSeconds         int
	NotFoundTTLSeconds int
}
