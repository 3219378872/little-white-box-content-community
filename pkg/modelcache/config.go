// Package modelcache defines the application's disposable model cache.
package modelcache

import "esx/pkg/redisstore"

// NodeConf is one Redis node of the model cache.
type NodeConf struct {
	redisstore.RedisConf
	Weight int
}

// CacheConf lists cache nodes; the current store accepts a single node or cluster entry.
type CacheConf []NodeConf

// Option adjusts cache TTLs when a model cache is created.
type Option func(*Options)

// Options sets row and not-found TTLs in seconds.
type Options struct {
	TTLSeconds         int
	NotFoundTTLSeconds int
}
