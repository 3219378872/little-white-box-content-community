package svc

import (
	"user/internal/config"
	"user/internal/model"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config            config.Config
	UserLoginLogModel model.UserLoginLogModel
	UserProfileModel  model.UserProfileModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewConn(sqlx.SqlConf{
		DataSource: c.DataSource,
		DriverName: "mysql",
	})
	if err != nil {
		logx.Errorf("数据库连接失败")
	}

	err = util.InitSnowflake(0, 1)
	if err != nil {
		logx.Errorf("雪花算法初始化失败%v", err)
	}

	return &ServiceContext{
		Config:            c,
		UserLoginLogModel: model.NewUserLoginLogModel(conn),
		UserProfileModel:  model.NewUserProfileModel(conn),
	}
}
