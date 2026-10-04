package svc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"esx/app/review/internal/cascade"
	"esx/app/review/internal/intake"
	"esx/app/review/internal/policy"
	"esx/app/review/internal/ranker"
	"esx/app/review/internal/router"
	"esx/app/review/internal/store"
	"esx/app/review/worker/internal/config"
	"esx/app/review/worker/internal/machine"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"

	logx "esx/pkg/logging"
)

type ServiceContext struct {
	Config      config.Config
	DB          *sql.DB
	Store       *store.Store
	Active      *policy.Policy
	Shadow      *policy.Policy
	Ingester    *intake.Ingester
	Processor   *machine.Processor
	OutboxRelay *outboxx.Relay
	Producer    *mqx.Producer
	Embedder    *router.GRPCEmbedder
	SeedIndex   *router.MilvusIndex
	ranker      *ranker.Client
}

func NewServiceContext(ctx context.Context, c config.Config) (*ServiceContext, error) {
	conn, err := sqlx.NewConn(sqlx.SqlConf{DataSource: c.DataSource, DriverName: "mysql"})
	if err != nil {
		return nil, fmt.Errorf("review worker database: %w", err)
	}
	rawDB, err := conn.RawDB()
	if err != nil {
		return nil, fmt.Errorf("review worker database access: %w", err)
	}
	if err := util.InitSnowflakeFromEnv(11, 1); err != nil {
		return nil, fmt.Errorf("review worker snowflake: %w", err)
	}
	// 政策配置加载失败时拒绝启动（DES-review-platform「失败模式」）。
	active, err := policy.Load(c.PolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("review worker active policy: %w", err)
	}
	var shadow *policy.Policy
	if c.ShadowPolicyVersion != "" {
		if shadow, err = policy.Load(c.ShadowPolicyVersion); err != nil {
			return nil, fmt.Errorf("review worker shadow policy: %w", err)
		}
	}
	reviewStore := store.New(conn)
	s := &ServiceContext{
		Config: c, DB: rawDB, Store: reviewStore, Active: active, Shadow: shadow,
		Ingester: &intake.Ingester{Store: reviewStore, Policy: active},
	}
	pipeline := &cascade.Cascade{Lookup: machine.LookupAdapter{Store: reviewStore}}
	if c.Ranker.Address != "" {
		client, err := ranker.Dial(c.Ranker.Address)
		if err != nil {
			return nil, err
		}
		s.ranker = client
		pipeline.Ranker = client
	}
	if r := s.connectRouter(ctx); r != nil {
		pipeline.Router = r
	}
	s.Processor = &machine.Processor{Store: reviewStore, Cascade: pipeline, Active: active, Shadow: shadow}
	if c.Producer.NameServer != "" {
		producer, err := mqx.NewProducer(c.Producer)
		if err != nil {
			return nil, fmt.Errorf("review worker producer: %w", err)
		}
		s.Producer = producer
		relay, err := outboxx.NewRelay(reviewStore.Outbox(), outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, sendErr := producer.Send(ctx, mqx.Message{Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload})
			return sendErr
		}), c.Outbox.RelayConfig("review-worker"))
		if err != nil {
			return nil, fmt.Errorf("review worker outbox relay: %w", err)
		}
		s.OutboxRelay = relay
	}
	return s, nil
}

// connectRouter 连接 embedding 与 Milvus；任一不可用时 Router 为空，级联按召回不可用降级（RVW-011）。
func (s *ServiceContext) connectRouter(ctx context.Context) cascade.RouterWithVersion {
	c := s.Config
	if c.Embedding.Address == "" || c.Milvus.Address == "" {
		logx.WithContext(ctx).Infow("review router disabled: embedding or milvus address not configured")
		return nil
	}
	embedder, err := router.DialEmbedder(c.Embedding.Address, c.Embedding.Dim)
	if err != nil {
		logx.Errorw("review router embedding unavailable", logx.Field("err", err.Error()))
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	index, err := router.DialMilvus(dialCtx, router.MilvusConfig{
		Address: c.Milvus.Address, Username: c.Milvus.Username, Password: c.Milvus.Password,
		Collection: c.Milvus.Collection, Dim: c.Embedding.Dim,
	})
	if err != nil {
		_ = embedder.Close()
		logx.Errorw("review router milvus unavailable", logx.Field("err", err.Error()))
		return nil
	}
	s.Embedder, s.SeedIndex = embedder, index
	return &router.Router{Embedder: embedder, Index: index, Authority: s.Store, TopK: s.Active.RouterTopK}
}

func (s *ServiceContext) Close() error {
	var errs []error
	if s.Producer != nil {
		errs = append(errs, s.Producer.Shutdown())
	}
	if s.ranker != nil {
		errs = append(errs, s.ranker.Close())
	}
	if s.Embedder != nil {
		errs = append(errs, s.Embedder.Close())
	}
	if s.SeedIndex != nil {
		errs = append(errs, s.SeedIndex.Close())
	}
	if s.DB != nil {
		errs = append(errs, s.DB.Close())
	}
	return errors.Join(errs...)
}
