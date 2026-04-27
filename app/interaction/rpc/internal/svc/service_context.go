package svc

import (
	"esx/app/interaction/rpc/internal/config"
	model2 "esx/app/interaction/rpc/internal/model"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"golang.org/x/sync/singleflight"
)

type ServiceContext struct {
	Config              config.Config
	Conn                sqlx.SqlConn
	FavoriteFolderModel model2.FavoriteFolderModel
	FavoriteModel       model2.FavoriteModel
	LikeRecordModel     model2.LikeRecordModel
	ReportModel         model2.ReportModel
	ViewHistoryModel    model2.ViewHistoryModel
	ActionCountModel    model2.ActionCountModel
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
		Conn:                conn,
		FavoriteFolderModel: model2.NewFavoriteFolderModel(conn, conf),
		FavoriteModel:       model2.NewFavoriteModel(conn, conf),
		LikeRecordModel:     model2.NewLikeRecordModel(conn, conf),
		ReportModel:         model2.NewReportModel(conn, conf),
		ViewHistoryModel:    model2.NewViewHistoryModel(conn, conf),
		ActionCountModel:    model2.NewActionCountModel(conn),
		Redis:               redisClient,
		RedisStore:          NewRedisStore(redisClient),
	}
}
