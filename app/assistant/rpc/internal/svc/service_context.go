package svc

import (
	"strings"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/safety"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/rpc/internal/config"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config         config.Config
	Store          store.Store
	Notify         store.Notifier
	Memory         memory.Store
	Watch          watch.Store
	Safety         safety.Filter
	Acceptor       *runtime.Acceptor
	ContentService contentservice.ContentService
	SearchService  searchservice.SearchService
	UserService    userservice.UserService
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
	internalAuthInterceptor := interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)
	newClient := func(conf zrpc.RpcClientConf) zrpc.Client {
		conf.Middlewares.Duration = false
		return zrpc.MustNewClient(conf,
			zrpc.WithUnaryClientInterceptor(bizErrInterceptor),
			zrpc.WithUnaryClientInterceptor(internalAuthInterceptor),
			zrpc.WithUnaryClientInterceptor(interceptor.SafeDurationUnaryClientInterceptor()),
		)
	}
	searchService := searchservice.NewSearchService(newClient(c.SearchRpc))
	contentService := contentservice.NewContentService(newClient(c.ContentRpc))
	userService := userservice.NewUserService(newClient(c.UserRpc))

	var st store.Store
	var mem memory.Store
	var watchStore watch.Store
	if strings.TrimSpace(c.DataSource) != "" {
		conn := sqlx.NewMysql(c.DataSource)
		st = store.NewSQLStore(conn)
		watchStore = watch.NewSQLStore(conn)
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
		Watch:          watchStore,
		Safety:         safetyFilter,
		Acceptor:       &runtime.Acceptor{Store: st, Memory: mem, Notify: notify, MaxRunes: c.MaxMessageRunes},
		ContentService: contentService,
		SearchService:  searchService,
		UserService:    userService,
	}
}

func (s *ServiceContext) WatchLookups() tool.Clients {
	return tool.Clients{Search: s.SearchService, Content: s.ContentService, User: s.UserService, Watch: s.Watch}
}
