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

type UserService interface {
	BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...callopt.Option) (*userservice.BatchGetUsersResp, error)
	SearchUsers(ctx context.Context, in *userservice.SearchUsersReq, opts ...callopt.Option) (*userservice.SearchUsersResp, error)
}

type ContentService interface {
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...callopt.Option) (*contentservice.GetPostsByIdsResp, error)
}

type ServiceContext struct {
	Config         config.Config
	Store          store.Store
	UserService    UserService
	ContentService ContentService
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateConfig(c); err != nil {
		panic(err)
	}
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
	return &ServiceContext{
		Config:         c,
		Store:          esStore,
		UserService:    userservice.NewUserService(userClient),
		ContentService: contentservice.NewContentService(contentClient),
	}
}

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
