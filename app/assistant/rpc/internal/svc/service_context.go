package svc

import (
	"strings"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/safety"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/config"
	"esx/app/content/rpc/contentservice"
	"esx/app/user/rpc/userservice"

	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

// ServiceContext 聚合 RPC 侧的存储、记忆、通知与输入接收器。
type ServiceContext struct {
	Config         config.Config
	Store          store.Store
	Memory         memory.Store
	Safety         safety.Filter
	Acceptor       *runtime.Acceptor
	ContentService contentservice.ContentService
	UserService    userservice.UserService
}

// NewServiceContext 装配 RPC 依赖；未配置 DataSource 时存储为空，相关接口返回 ServiceUnavailable。
// 屏蔽词过滤器配置无效时返回错误让启动失败，与 worker 一致，而不是静默关闭过滤。
func NewServiceContext(c config.Config) (*ServiceContext, error) {
	safetyFilter, err := newSafetyFilter(c.Safety)
	if err != nil {
		return nil, err
	}

	internalAuthOption := rpcx.WithInternalAuth(c.InternalSecret)
	newClient := func(conf rpcx.RpcClientConf) rpcx.Client {
		return rpcx.MustNewClient(conf,

			internalAuthOption,
		)
	}
	contentService := contentservice.NewContentService(newClient(c.ContentRpc))
	userService := userservice.NewUserService(newClient(c.UserRpc))

	var st store.Store
	var mem memory.Store
	if strings.TrimSpace(c.DataSource) != "" {
		// pkg/sqlstore never logs statements or arguments, so prompts and tool payloads stay out of logs (REL-022).
		conn := sqlx.NewMysql(c.DataSource)
		st = store.NewSQLStore(conn)
		mem = memory.NewSQLStore(conn, safetyFilter)
	}
	return &ServiceContext{
		Config:         c,
		Store:          st,
		Memory:         mem,
		Safety:         safetyFilter,
		Acceptor:       &runtime.Acceptor{Store: st, Memory: mem, MaxRunes: c.MaxMessageRunes},
		ContentService: contentService,
		UserService:    userService,
	}, nil
}

// newSafetyFilter 按配置构造屏蔽词过滤器；关闭时返回 nil，启用但配置无效时返回错误。
func newSafetyFilter(c config.SafetyConfig) (safety.Filter, error) {
	if !c.Enabled {
		return nil, nil
	}
	filter, err := safety.NewKeywordFilter(c.BlockedTerms, c.MaxScanRunes)
	if err != nil {
		return nil, err
	}
	return filter, nil
}
