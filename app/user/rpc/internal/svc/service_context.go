package svc

import (
	"context"
	"database/sql"
	"errors"
	"esx/pkg/outboxx"
	"fmt"
	"mqx"
	"user/internal/config"
	"user/internal/model"
	"util"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type UserProfileStore interface {
	FindOne(ctx context.Context, id int64) (*model.UserProfile, error)
	FindByIDs(ctx context.Context, ids []int64) ([]*model.UserProfile, error)
	FindOneByPhone(ctx context.Context, phone sql.NullString) (*model.UserProfile, error)
	FindOneByUsername(ctx context.Context, username string) (*model.UserProfile, error)
	Insert(ctx context.Context, data *model.UserProfile) (sql.Result, error)
	UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error
	SearchPublic(ctx context.Context, keyword string, offset, limit int64) ([]*model.UserProfile, int64, error)
}

type UserFollowStore interface {
	Follow(ctx context.Context, userID, targetUserID int64) error
	Unfollow(ctx context.Context, userID, targetUserID int64) error
	FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
	FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
	CountFollowers(ctx context.Context, userID int64) (int64, error)
	CountFollowing(ctx context.Context, userID int64) (int64, error)
}

type UserFollowCommandStore interface {
	Follow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
	Unfollow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
}

// RedisStore 是 user 服务使用的 Redis 能力子集，便于测试注入。
type RedisStore interface {
	GetCtx(ctx context.Context, key string) (string, error)
	DelCtx(ctx context.Context, keys ...string) (int, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
}

type ServiceContext struct {
	Config             config.Config
	DB                 sqlx.SqlConn
	RawDB              *sql.DB
	UserLoginLogModel  model.UserLoginLogModel
	UserProfileModel   UserProfileStore
	UserFollowModel    UserFollowStore
	UserFollowCommands UserFollowCommandStore
	Personalization    model.PersonalizationPreferenceStore
	RedisClient        RedisStore
	OutboxStore        *outboxx.SQLStore
	OutboxRelay        *outboxx.Relay
	MQProducer         *mqx.Producer
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
	rawDB, err := conn.RawDB()
	if err != nil {
		panic(fmt.Sprintf("数据库连接访问失败: %v", err))
	}

	// 注入Redis
	newRedis := redis.MustNewRedis(c.Redis.RedisConf)

	// 初始化雪花算法
	err = util.InitSnowflake(0, 1)
	if err != nil {
		panic(fmt.Sprintf("雪花算法初始化失败: %v", err))
	}

	outboxStore := outboxx.NewSQLStore(conn)
	var producer *mqx.Producer
	var outboxRelay *outboxx.Relay
	if c.MQ.NameServer != "" {
		producer, err = mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("user RocketMQ producer initialization failed: %w", err))
		}
		outboxRelay, err = outboxx.NewRelay(outboxStore, outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, publishErr := producer.Send(ctx, mqx.Message{
				Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload,
			})
			return publishErr
		}), c.Outbox.RelayConfig("user"))
		if err != nil {
			panic(fmt.Errorf("user outbox relay initialization failed: %w", err))
		}
	}
	followModel := model.NewUserFollowModel(conn)

	return &ServiceContext{
		Config:             c,
		DB:                 conn,
		RawDB:              rawDB,
		UserLoginLogModel:  model.NewUserLoginLogModel(conn),
		UserProfileModel:   model.NewUserProfileModel(conn),
		UserFollowModel:    followModel,
		UserFollowCommands: model.NewUserFollowCommandModel(conn, outboxStore),
		Personalization:    model.NewPersonalizationPreferenceModel(conn),
		RedisClient:        newRedis,
		OutboxStore:        outboxStore,
		OutboxRelay:        outboxRelay,
		MQProducer:         producer,
	}
}

func (s *ServiceContext) RunOutboxRelay(ctx context.Context) error {
	if s == nil || s.OutboxRelay == nil {
		return nil
	}
	return s.OutboxRelay.Run(ctx)
}

func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	var closeErrors []error
	if s.MQProducer != nil {
		closeErrors = append(closeErrors, s.MQProducer.Shutdown())
	}
	if s.RawDB != nil {
		closeErrors = append(closeErrors, s.RawDB.Close())
	}
	return errors.Join(closeErrors...)
}
