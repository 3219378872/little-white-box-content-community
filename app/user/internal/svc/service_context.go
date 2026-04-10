package svc

import (
	"user/internal/config"
	"user/internal/model"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
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
		logx.Errorf("数据库连接失败")
		panic("数据库初始化失败")
	}

	// 注入Redis
	newRedis := redis.MustNewRedis(c.Redis.RedisConf)

	// 初始化雪花算法
	err = util.InitSnowflake(0, 1)
	if err != nil {
		logx.Errorf("雪花算法初始化失败%v", err)
		panic("雪花算法初始化失败")
	}

	return &ServiceContext{
		Config:            c,
		DB:                conn,
		UserLoginLogModel: model.NewUserLoginLogModel(conn),
		UserProfileModel:  model.NewUserProfileModel(conn),
		RedisClient:       newRedis,
	}
}
