package svc

import (
	"esx/app/interaction/internal/config"
	"esx/app/interaction/internal/model"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"golang.org/x/sync/singleflight"
)

type ServiceContext struct {
	Config              config.Config
	FavoriteFolderModel model.FavoriteFolderModel
	FavoriteModel       model.FavoriteModel
	LikeRecordModel     model.LikeRecordModel
	ReportModel         model.ReportModel
	ViewHistoryModel    model.ViewHistoryModel
	ActionCountModel    model.ActionCountModel
	Redis               *redis.Redis
	RedisStore          RedisStore
	SingleFlight        singleflight.Group
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewConn(sqlx.SqlConf{
		DataSource: c.DataSource,
		DriverName: "mysql",
	})
	if err != nil {
		panic("数据库连接初始化错误")
	}

	conf := cache.CacheConf{
		cache.NodeConf{
			RedisConf: c.Redis.RedisConf,
			Weight:    100,
		},
	}

	redisClient := redis.MustNewRedis(c.Redis.RedisConf)

	return &ServiceContext{
		Config:              c,
		FavoriteFolderModel: model.NewFavoriteFolderModel(conn, conf),
		FavoriteModel:       model.NewFavoriteModel(conn, conf),
		LikeRecordModel:     model.NewLikeRecordModel(conn, conf),
		ReportModel:         model.NewReportModel(conn, conf),
		ViewHistoryModel:    model.NewViewHistoryModel(conn, conf),
		ActionCountModel:    model.NewActionCountModel(conn),
		Redis:               redisClient,
		RedisStore:          NewRedisStore(redisClient),
	}
}
