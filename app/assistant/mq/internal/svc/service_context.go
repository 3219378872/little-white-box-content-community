package svc

import (
	"fmt"
	"strings"

	"esx/app/assistant/mq/internal/config"
	"esx/app/assistant/watch"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config config.Config
	Watch  watch.Store
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil, fmt.Errorf("assistant-watch-matcher: DataSource is required")
	}
	return &ServiceContext{
		Config: c,
		Watch:  watch.NewSQLStore(sqlx.NewMysql(c.DataSource)),
	}, nil
}
