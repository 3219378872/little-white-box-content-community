package config

import (
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/zrpc"
)

type OnlineInferConfig struct {
	Enabled      bool
	Rpc          zrpc.RpcClientConf
	ModelVersion string `json:",default=auto"`
	TimeoutMs    int64  `json:",default=80,range=[1:1000]"`
}

type ElasticsearchRecallConfig struct {
	Enabled   bool
	Addresses []string
	Index     string
	Username  string `json:",optional"`
	Password  string `json:",optional"`
	TimeoutMs int64  `json:",default=120,range=[1:5000]"`
}

type MilvusRecallConfig struct {
	Enabled    bool
	Address    string
	Collection string
	Username   string `json:",optional"`
	Password   string `json:",optional"`
	Database   string `json:",optional"`
	NProbe     int    `json:",default=16,range=[1:1024]"`
	TimeoutMs  int64  `json:",default=120,range=[1:5000]"`
}

type Config struct {
	zrpc.RpcServerConf
	ContentRpc          zrpc.RpcClientConf
	UserRpc             zrpc.RpcClientConf
	FeatureVersion      string `json:",default=v2"`
	RecallKeyPrefix     string `json:",default=recommend"`
	DefaultPageSize     int    `json:",default=20,range=[1:100]"`
	MaxPageSize         int    `json:",default=50,range=[1:100]"`
	CandidateMultiplier int    `json:",default=8,range=[2:20]"`
	CursorSecret        string
	CursorTTLSeconds    int     `json:",default=600,range=[60:3600]"`
	RuleModelVersion    string  `json:",default=rules-v2"`
	ExploreRatio        float64 `json:",default=0.1"`
	MaxPerAuthor        int     `json:",default=2,range=[1:20]"`
	OnlineInfer         OnlineInferConfig
	ElasticsearchRecall ElasticsearchRecallConfig
	MilvusRecall        MilvusRecallConfig
}

func (c Config) Validate() error {
	if len(c.CursorSecret) < 32 {
		return fmt.Errorf("recommend CursorSecret must be at least 32 bytes")
	}
	if c.MaxPageSize < c.DefaultPageSize {
		return fmt.Errorf("recommend MaxPageSize must be greater than or equal to DefaultPageSize")
	}
	if c.ExploreRatio < 0 || c.ExploreRatio > 0.5 {
		return fmt.Errorf("recommend ExploreRatio must be between 0 and 0.5")
	}
	if c.OnlineInfer.Enabled {
		if len(c.OnlineInfer.Rpc.Endpoints) == 0 || strings.TrimSpace(c.OnlineInfer.Rpc.Endpoints[0]) == "" ||
			strings.TrimSpace(c.OnlineInfer.ModelVersion) == "" || c.OnlineInfer.TimeoutMs <= 0 {
			return fmt.Errorf("recommend OnlineInfer configuration is incomplete")
		}
	}
	if c.ElasticsearchRecall.Enabled {
		if len(c.ElasticsearchRecall.Addresses) == 0 || strings.TrimSpace(c.ElasticsearchRecall.Addresses[0]) == "" ||
			strings.TrimSpace(c.ElasticsearchRecall.Index) == "" || c.ElasticsearchRecall.TimeoutMs <= 0 {
			return fmt.Errorf("recommend ElasticsearchRecall configuration is incomplete")
		}
	}
	if c.MilvusRecall.Enabled {
		if strings.TrimSpace(c.MilvusRecall.Address) == "" || strings.TrimSpace(c.MilvusRecall.Collection) == "" ||
			c.MilvusRecall.NProbe <= 0 || c.MilvusRecall.TimeoutMs <= 0 {
			return fmt.Errorf("recommend MilvusRecall configuration is incomplete")
		}
	}
	return nil
}
