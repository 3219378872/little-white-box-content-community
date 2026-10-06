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

	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

// ServiceContext 聚合 RPC 侧的存储、记忆、通知与输入接收器。
type ServiceContext struct {
	Config         config.Config
	Store          store.Store
	Notify         store.Notifier
	Memory         memory.Store
	Safety         safety.Filter
	Acceptor       *runtime.Acceptor
	ContentService contentservice.ContentService
	UserService    userservice.UserService
}

// NewServiceContext 装配 RPC 依赖；未配置 DataSource 时存储为空，相关接口返回 ServiceUnavailable。
func NewServiceContext(c config.Config) *ServiceContext {

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
		var filter safety.Filter
		if c.Safety.Enabled {
			f, err := safety.NewKeywordFilter(c.Safety.BlockedTerms, c.Safety.MaxScanRunes)
			if err == nil {
				filter = f
			}
		}
		mem = memory.NewSQLStore(conn, filter)
	}
	var safetyFilter safety.Filter
	if c.Safety.Enabled {
		f, err := safety.NewKeywordFilter(c.Safety.BlockedTerms, c.Safety.MaxScanRunes)
		if err == nil {
			safetyFilter = f
		}
	}
	redisClient := redis.MustNewRedis(c.Redis.RedisConf)
	notify := store.NewRedisNotifier(redisClient)
	return &ServiceContext{
		Config:         c,
		Store:          st,
		Notify:         notify,
		Memory:         mem,
		Safety:         safetyFilter,
		Acceptor:       &runtime.Acceptor{Store: st, Memory: mem, Notify: notify, MaxRunes: c.MaxMessageRunes},
		ContentService: contentService,
		UserService:    userService,
	}
}
