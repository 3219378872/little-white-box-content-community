// Package rpcx owns Kitex service discovery, transport policy and authentication.
package rpcx

import (
	"esx/pkg/lifecycle"
	"esx/pkg/redisstore"
)

// EtcdConf locates the service registry.
type EtcdConf struct {
	Hosts      []string
	Key        string
	User, Pass string
}

// RpcClientConf targets a service through etcd discovery or fixed endpoints; Timeout is in milliseconds.
type RpcClientConf struct {
	Etcd      EtcdConf
	Endpoints []string
	Target    string
	NonBlock  bool
	Timeout   int64 `json:",default=2000"`
}

// RpcServerConf configures a Kitex gRPC server, its registry entry and its admission limits.
type RpcServerConf struct {
	lifecycle.ServiceConf
	ListenOn       string
	Etcd           EtcdConf
	Redis          redisstore.RedisKeyConf
	Timeout        int64 `json:",default=2000"`
	Health         bool  `json:",default=true"`
	MaxConnections int   `json:",default=10000"`
	MaxQPS         int   `json:",default=10000"`
}

const RegistryPrefix = "/little/kitex"
