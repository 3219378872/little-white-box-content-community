package svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/internal/config"
	"esx/app/search/rpc/internal/store"
	"esx/app/user/rpc/userservice"

	"esx/pkg/rpcx"
)

// UserService 是搜索用到的用户服务子集：作者卡片补全与用户搜索。
type UserService interface {
	BatchGetUserCards(ctx context.Context, in *userservice.BatchGetUserCardsReq, opts ...callopt.Option) (*userservice.BatchGetUserCardsResp, error)
	SearchUsers(ctx context.Context, in *userservice.SearchUsersReq, opts ...callopt.Option) (*userservice.SearchUsersResp, error)
}

// ContentService 是搜索用到的内容服务子集，用于以权威状态过滤不可见帖子。
type ContentService interface {
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...callopt.Option) (*contentservice.GetPostsByIdsResp, error)
}

// ServiceContext 持有搜索 RPC 的配置、存储与下游服务。
type ServiceContext struct {
	Config         config.Config
	Store          store.Store
	UserService    UserService
	ContentService ContentService
}

// NewServiceContext 装配搜索依赖；ES 索引不可用时 panic，避免服务启动后所有搜索都失败。
func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateConfig(c); err != nil {
		panic(err)
	}
	// 在启动超时内确认索引存在，ES 不可达时不对外提供服务。
	esStore, err := store.NewElasticsearchStore(c.ES.Addresses, c.ES.Index,
		store.WithBasicAuth(c.ES.Username, c.ES.Password))
	if err != nil {
		panic(fmt.Errorf("search-rpc: initialize Elasticsearch: %w", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.ES.StartupTimeoutMillis)*time.Millisecond)
	defer cancel()
	if err := esStore.Health(ctx); err != nil {
		panic(fmt.Errorf("search-rpc: Elasticsearch health check: %w", err))
	}
	userClient := rpcx.MustNewClient(c.UserRpc, rpcx.WithInternalAuth(c.InternalSecret))
	contentClient := rpcx.MustNewClient(c.ContentRpc, rpcx.WithInternalAuth(c.InternalSecret))
	// 标签搜索经进程内缓存，其余读请求直达 ES。
	return &ServiceContext{
		Config:         c,
		Store:          store.NewTagCache(esStore, store.TagCacheTTL, store.TagCacheCapacity),
		UserService:    userservice.NewUserService(userClient),
		ContentService: contentservice.NewContentService(contentClient),
	}
}

// validateConfig 一次列出所有缺失的 ES 配置项，便于部署时一次修正。
func validateConfig(c config.Config) error {
	missing := make([]string, 0, 3)
	if len(c.ES.Addresses) == 0 || strings.TrimSpace(c.ES.Addresses[0]) == "" {
		missing = append(missing, "ES.Addresses")
	}
	if strings.TrimSpace(c.ES.Index) == "" {
		missing = append(missing, "ES.Index")
	}
	if c.ES.StartupTimeoutMillis <= 0 {
		missing = append(missing, "ES.StartupTimeoutMillis")
	}
	if len(missing) > 0 {
		return fmt.Errorf("search-rpc: invalid config: %s", strings.Join(missing, ", "))
	}
	return nil
}
