package svc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/app/ad/mq/internal/apply"
	"esx/app/ad/mq/internal/config"
	"esx/app/review/rpc/reviewservice"
	"esx/pkg/event"
	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

type ServiceContext struct {
	Config     config.Config
	DB         *sql.DB
	Applier    *apply.Applier
	Reconciler *apply.Reconciler
	Rescanner  *apply.Rescanner
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	conn, err := sqlx.NewConn(sqlx.SqlConf{DataSource: c.DataSource, DriverName: "mysql"})
	if err != nil {
		return nil, fmt.Errorf("ad-mq database: %w", err)
	}
	rawDB, err := conn.RawDB()
	if err != nil {
		return nil, fmt.Errorf("ad-mq database access: %w", err)
	}
	if err := util.InitSnowflakeFromEnv(13, 1); err != nil {
		return nil, fmt.Errorf("ad-mq snowflake: %w", err)
	}
	storage, err := assets.New(context.Background(), assets.Config{
		Endpoint: c.Assets.Endpoint, AccessKey: c.Assets.AccessKey, SecretKey: c.Assets.SecretKey,
		UseSSL: c.Assets.UseSSL, Region: c.Assets.Region, PrivateBucket: c.Assets.PrivateBucket,
		PublicBucket: c.Assets.PublicBucket, PublicBaseURL: c.Assets.PublicBaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("ad-mq asset storage: %w", err)
	}
	kv, err := redis.NewRedis(c.Redis)
	if err != nil {
		return nil, fmt.Errorf("ad-mq redis: %w", err)
	}
	adStore := store.New(conn)
	server := &apply.Server{Store: adStore, Publisher: storage, Index: serving.Index{KV: kv}}
	review := reviewservice.NewReviewService(rpcx.MustNewClient(c.ReviewRpc, rpcx.WithInternalAuth(c.InternalSecret)))
	return &ServiceContext{
		Config: c, DB: rawDB,
		Applier:    &apply.Applier{Store: adStore, Server: server},
		Reconciler: &apply.Reconciler{Store: adStore, Review: ensurer{review}, Server: server},
		Rescanner:  &apply.Rescanner{Store: adStore, Source: generations{review}},
	}, nil
}

func (s *ServiceContext) Close() error {
	if s.DB != nil {
		return errors.Join(s.DB.Close())
	}
	return nil
}

type ensurer struct{ client reviewservice.ReviewService }

func (e ensurer) EnsureSubmitted(ctx context.Context, sub event.ReviewSubmittedEvent) (int64, bool, error) {
	raw, err := json.Marshal(sub)
	if err != nil {
		return 0, false, err
	}
	resp, err := e.client.EnsureSubmitted(ctx, &reviewservice.EnsureSubmittedReq{SubmissionJson: string(raw)})
	if err != nil {
		return 0, false, err
	}
	return resp.GetTaskId(), resp.GetCreated(), nil
}

type generations struct{ client reviewservice.ReviewService }

func (g generations) RescanGeneration(ctx context.Context) (apply.Generation, error) {
	resp, err := g.client.GetRescanGeneration(ctx, &reviewservice.GetRescanGenerationReq{})
	if err != nil {
		return apply.Generation{}, err
	}
	return apply.Generation{Ready: resp.GetReady(), Value: resp.GetGeneration(), EffectiveSinceMs: resp.GetEffectiveSinceMs()}, nil
}
