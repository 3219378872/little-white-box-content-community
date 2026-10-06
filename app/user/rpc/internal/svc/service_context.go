package svc

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/user/rpc/internal/config"
	"esx/app/user/rpc/internal/model"
	cache "esx/pkg/modelcache"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
	"fmt"

	redis "esx/pkg/redisstore"
	sqlx "esx/pkg/sqlstore"
)

// UserProfileStore 是用户资料的读写接口。
type UserProfileStore interface {
	FindOne(ctx context.Context, id int64) (*model.UserProfile, error)
	FindByIDs(ctx context.Context, ids []int64) ([]*model.UserProfile, error)
	FindCardsByIDs(ctx context.Context, ids []int64) ([]*model.UserCard, error)
	FindOneByPhone(ctx context.Context, phone sql.NullString) (*model.UserProfile, error)
	FindOneByUsername(ctx context.Context, username string) (*model.UserProfile, error)
	Insert(ctx context.Context, data *model.UserProfile) (sql.Result, error)
	UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error
	SearchPublic(ctx context.Context, keyword string, offset, limit int64) ([]*model.UserProfile, int64, error)
	SearchPublicPage(ctx context.Context, keyword string, offset, limit int64) ([]*model.UserProfile, error)
}

// UserFollowStore 是关注关系的查询接口。
type UserFollowStore interface {
	FindOneByUserIdTargetUserId(ctx context.Context, userID, targetUserID int64) (*model.UserFollow, error)
	Follow(ctx context.Context, userID, targetUserID int64) error
	Unfollow(ctx context.Context, userID, targetUserID int64) error
	FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
	FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
	CountFollowers(ctx context.Context, userID int64) (int64, error)
	CountFollowing(ctx context.Context, userID int64) (int64, error)
}

// UserFollowCommandStore 是关注/取关的事务写入，关系、计数与行为事件一起提交。
type UserFollowCommandStore interface {
	Follow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
	Unfollow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
}

// UserTagStore 是 GetUserTags 逻辑依赖的标签读取能力。
type UserTagStore interface {
	FindByUserId(ctx context.Context, userID int64) ([]*model.UserTag, error)
}

// RedisStore 是 user 服务使用的 Redis 能力子集，便于测试注入。
type RedisStore interface {
	GetCtx(ctx context.Context, key string) (string, error)
	DelCtx(ctx context.Context, keys ...string) (int, error)
	EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error)
	IncrCtx(ctx context.Context, key string) (int64, error)
	ExpireCtx(ctx context.Context, key string, seconds int) error
}

// ServiceContext 持有用户 RPC 的存储、Redis 与 outbox 投递依赖。
type ServiceContext struct {
	Config             config.Config
	DB                 sqlx.SqlConn
	RawDB              *sql.DB
	UserProfileModel   UserProfileStore
	UserFollowModel    UserFollowStore
	UserFollowCommands UserFollowCommandStore
	UserTagModel       UserTagStore
	Personalization    model.PersonalizationPreferenceStore
	AgentConsent       model.AgentCapabilityConsentStore
	RedisClient        RedisStore
	OutboxStore        *outboxx.SQLStore
	OutboxRelay        *outboxx.Relay
	MQProducer         *mqx.Producer
}

// NewServiceContext 装配数据库、Redis、ID 生成器与各模型；未配置 MQ 时不创建 relay。
func NewServiceContext(c config.Config) *ServiceContext {
	// SQL 参数含手机号与私密资料；pkg/sqlstore 不记录 SQL 语句与参数（REL-022）。
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
	err = util.InitSnowflakeFromEnv(0, 1)
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
		Config: c,
		DB:     conn,
		RawDB:  rawDB,
		UserProfileModel: model.NewCachedUserProfileModel(conn, cache.CacheConf{
			cache.NodeConf{RedisConf: c.Redis.RedisConf, Weight: 100},
		}),
		UserFollowModel:    followModel,
		UserFollowCommands: model.NewUserFollowCommandModel(conn, outboxStore),
		UserTagModel:       model.NewUserTagModel(conn),
		Personalization:    model.NewPersonalizationPreferenceModel(conn),
		AgentConsent:       model.NewAgentCapabilityConsentModel(conn),
		RedisClient:        newRedis,
		OutboxStore:        outboxStore,
		OutboxRelay:        outboxRelay,
		MQProducer:         producer,
	}
}

// Close 关闭 MQ 生产者与数据库连接，汇总所有关闭错误。
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
