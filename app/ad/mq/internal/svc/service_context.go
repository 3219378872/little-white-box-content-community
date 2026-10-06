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

// ServiceContext 持有广告异步处理的结论应用、送审对账与复扫组件。
type ServiceContext struct {
	Config     config.Config
	DB         *sql.DB
	Applier    *apply.Applier
	Reconciler *apply.Reconciler
	Rescanner  *apply.Rescanner
}

// NewServiceContext 装配数据库、对象存储、Redis 投放索引与审核服务客户端。
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

// Close 关闭数据库连接。
func (s *ServiceContext) Close() error {
	if s.DB != nil {
		return errors.Join(s.DB.Close())
	}
	return nil
}

// ensurer 通过审核 RPC 确认送审任务存在。
type ensurer struct{ client reviewservice.ReviewService }

// EnsureSubmitted 提交送审事件，返回任务 ID 以及本次是否补建。
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

// generations 通过审核 RPC 读取当前复扫规则代际。
type generations struct{ client reviewservice.ReviewService }

// RescanGeneration 返回当前规则代际及其生效时间。
func (g generations) RescanGeneration(ctx context.Context) (apply.Generation, error) {
	resp, err := g.client.GetRescanGeneration(ctx, &reviewservice.GetRescanGenerationReq{})
	if err != nil {
		return apply.Generation{}, err
	}
	return apply.Generation{Ready: resp.GetReady(), Value: resp.GetGeneration(), EffectiveSinceMs: resp.GetEffectiveSinceMs()}, nil
}
