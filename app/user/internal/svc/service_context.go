package svc

import (
	"fmt"
	"user/internal/config"
	"user/internal/model"
	"util"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config            config.Config
	DB                sqlx.SqlConn
	UserLoginLogModel model.UserLoginLogModel
	UserProfileModel  model.UserProfileModel
	RedisClient       *redis.Redis
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 注入MySQL
	conn, err := sqlx.NewConn(sqlx.SqlConf{
		DataSource: c.DataSource,
		DriverName: "mysql",
	})
	if err != nil {
		panic(fmt.Sprintf("数据库初始化失败: %v", err))
	}

	// 注入Redis
	newRedis := redis.MustNewRedis(c.Redis.RedisConf)

	// 初始化雪花算法
	err = util.InitSnowflake(0, 1)
	if err != nil {
		panic(fmt.Sprintf("雪花算法初始化失败: %v", err))
	}

	return &ServiceContext{
		Config:            c,
		DB:                conn,
		UserLoginLogModel: model.NewUserLoginLogModel(conn),
		UserProfileModel:  model.NewUserProfileModel(conn),
		RedisClient:       newRedis,
	}
}
