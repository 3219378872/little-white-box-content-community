package svc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"esx/app/review/internal/intake"
	"esx/app/review/internal/policy"
	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/config"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

type ServiceContext struct {
	Config      config.Config
	DB          *sql.DB
	Store       Store
	Policy      *policy.Policy
	Ingester    Ingester
	OutboxRelay *outboxx.Relay
	MQProducer  *mqx.Producer
	Clock       func() time.Time
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewConn(sqlx.SqlConf{DataSource: c.DataSource, DriverName: "mysql"})
	if err != nil {
		panic(fmt.Sprintf("review rpc database initialization failed: %v", err))
	}
	rawDB, err := conn.RawDB()
	if err != nil {
		panic(fmt.Sprintf("review rpc database access failed: %v", err))
	}
	if err := util.InitSnowflakeFromEnv(10, 1); err != nil {
		panic(fmt.Sprintf("review rpc snowflake initialization failed: %v", err))
	}
	// 政策加载失败时拒绝启动，不使用旧缓存猜测。
	active, err := policy.Load(c.PolicyVersion)
	if err != nil {
		panic(fmt.Sprintf("review rpc policy %q: %v", c.PolicyVersion, err))
	}
	reviewStore := store.New(conn)
	var producer *mqx.Producer
	var relay *outboxx.Relay
	if c.MQ.NameServer != "" {
		producer, err = mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("review rpc RocketMQ producer initialization failed: %w", err))
		}
		relay, err = outboxx.NewRelay(reviewStore.Outbox(), outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, sendErr := producer.Send(ctx, mqx.Message{Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload})
			return sendErr
		}), c.Outbox.RelayConfig("review-rpc"))
		if err != nil {
			panic(fmt.Errorf("review rpc outbox relay initialization failed: %w", err))
		}
	}
	return &ServiceContext{
		Config: c, DB: rawDB, Store: reviewStore, Policy: active,
		Ingester:    &intake.Ingester{Store: reviewStore, Policy: active},
		OutboxRelay: relay, MQProducer: producer, Clock: time.Now,
	}
}

func (s *ServiceContext) Now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.MQProducer != nil {
		errs = append(errs, s.MQProducer.Shutdown())
	}
	if s.DB != nil {
		errs = append(errs, s.DB.Close())
	}
	return errors.Join(errs...)
}
