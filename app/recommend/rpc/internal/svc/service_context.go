package svc

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"
	inferencepb "esx/app/recommend/rpc/xiaobaihe/inference/pb"
	"esx/app/user/rpc/userservice"

	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
)

type ServiceContext struct {
	Config             config.Config
	ContentService     contentservice.ContentService
	UserService        userservice.UserService
	PostRecallSources  []model.PostRecallSource
	SimilarPostSources []model.PostRecallSource
	UserRecallSources  []model.UserRecallSource
	FeatureRepository  model.FeatureRepository
	SnapshotStore      model.SnapshotStore
	InferenceRanker    model.InferenceRanker
	CursorCodec        *cursor.Codec
	Now                func() time.Time
	NewSnapshotID      func() (string, error)
	closers            []io.Closer
}

// NewServiceContext 按依赖顺序装配推荐服务：配置校验 → 游标编解码与 Redis → 下游 RPC 客户端 →
// 召回来源 → 特征、快照与可选的在线推理。可选组件持有的连接登记到 closers，由 Close 统一释放。
func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	now := time.Now
	cursorCodec, err := cursor.New(c.CursorSecret, now)
	if err != nil {
		return nil, err
	}
	redisClient, err := redis.NewRedis(c.Redis.RedisConf)
	if err != nil {
		return nil, fmt.Errorf("initialize recommend redis: %w", err)
	}
	contentService, userService, err := newDownstreamServices(c)
	if err != nil {
		return nil, err
	}
	prefix := c.RecallKeyPrefix + ":" + c.FeatureVersion
	sources, err := newRecallSources(c, prefix, redisClient, contentService, userService)
	if err != nil {
		return nil, err
	}
	serviceContext := &ServiceContext{
		Config:             c,
		ContentService:     contentService,
		UserService:        userService,
		PostRecallSources:  sources.posts,
		SimilarPostSources: sources.similar,
		UserRecallSources:  sources.users,
		FeatureRepository:  model.NewRedisFeatureRepository(redisClient, c.FeatureVersion, userService),
		SnapshotStore:      model.NewRedisSnapshotStore(redisClient, prefix),
		CursorCodec:        cursorCodec,
		Now:                now,
		NewSnapshotID:      randomSnapshotID,
		closers:            sources.closers,
	}
	// 在线推理是可选的精排；未启用时排序只用规则分。
	if c.OnlineInfer.Enabled {
		client, err := rpcx.NewGRPCClient(c.OnlineInfer.Rpc)
		if err != nil {
			return nil, fmt.Errorf("initialize online inference client: %w", err)
		}
		serviceContext.closers = append(serviceContext.closers, client)
		serviceContext.InferenceRanker = model.NewGRPCInferenceRanker(inferencepb.NewOnlineInferServiceClient(client))
	}
	return serviceContext, nil
}

// newDownstreamServices 创建 content 与 user 的 RPC 客户端。
func newDownstreamServices(c config.Config) (contentservice.ContentService, userservice.UserService, error) {
	// content/user 的服务端挂了内部签名校验拦截器，出站必须同样签名；
	// 否则请求被 Unauthenticated 拒绝并经 errx 映射成 1006，推荐整体降级到规则。
	internalAuthOption := rpcx.WithInternalAuth(c.InternalSecret)
	contentClient, err := rpcx.NewClient(c.ContentRpc, internalAuthOption)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize content rpc client: %w", err)
	}
	userClient, err := rpcx.NewClient(c.UserRpc, internalAuthOption)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize user rpc client: %w", err)
	}
	return contentservice.NewContentService(contentClient), userservice.NewUserService(userClient), nil
}

// recallSources 是装配好的三组召回来源，以及需要随服务关闭的连接。
type recallSources struct {
	posts   []model.PostRecallSource
	similar []model.PostRecallSource
	users   []model.UserRecallSource
	closers []io.Closer
}

// newRecallSources 组装召回来源。各来源结果按 RRF 融合，同一帖子保留最先出现来源的理由，
// 所以 Redis 个性化与热门来源在前、可选的 ES / Milvus 向量召回其次；内容服务兜底来源放最后，
// 保证 Redis 特征冷启动时仍有候选。
func newRecallSources(
	c config.Config,
	prefix string,
	redisClient *redis.Redis,
	contentService contentservice.ContentService,
	userService userservice.UserService,
) (recallSources, error) {
	sources := recallSources{
		posts: []model.PostRecallSource{
			model.NewRedisPostRecallSource("follow", "from followed creators", redisClient, identityPostKey(prefix, "follow")),
			model.NewRedisPostRecallSource("hot", "popular now", redisClient, scenePostKey(prefix, "hot")),
			model.NewRedisPostRecallSource("explore", "explore something new", redisClient, scenePostKey(prefix, "explore")),
			model.NewRedisPostRecallSource("itemcf", "based on recent interests", redisClient, identityPostKey(prefix, "itemcf")),
		},
		similar: []model.PostRecallSource{
			model.NewRedisPostRecallSource("itemcf", "people also engaged with", redisClient, similarPostKey(prefix, "itemcf")),
		},
		closers: make([]io.Closer, 0, 1),
	}
	// 外部向量召回同时服务首页推荐与相似帖。
	if c.ElasticsearchRecall.Enabled {
		esRecall, err := model.NewElasticsearchPostRecallSource(model.ElasticsearchRecallOptions{
			Addresses: c.ElasticsearchRecall.Addresses, Index: c.ElasticsearchRecall.Index,
			Username: c.ElasticsearchRecall.Username, Password: c.ElasticsearchRecall.Password,
			FeatureVersion: c.FeatureVersion, Redis: redisClient,
			Timeout: time.Duration(c.ElasticsearchRecall.TimeoutMs) * time.Millisecond,
		})
		if err != nil {
			return recallSources{}, err
		}
		sources.posts = append(sources.posts, esRecall)
		sources.similar = append(sources.similar, esRecall)
	}
	if c.MilvusRecall.Enabled {
		milvusRecall := model.NewMilvusPostRecallSource(model.MilvusRecallOptions{
			Address: c.MilvusRecall.Address, Collection: c.MilvusRecall.Collection,
			Username: c.MilvusRecall.Username, Password: c.MilvusRecall.Password, Database: c.MilvusRecall.Database,
			FeatureVersion: c.FeatureVersion, NProbe: c.MilvusRecall.NProbe, Redis: redisClient,
			Timeout: time.Duration(c.MilvusRecall.TimeoutMs) * time.Millisecond,
		})
		sources.posts = append(sources.posts, milvusRecall)
		sources.similar = append(sources.similar, milvusRecall)
		sources.closers = append(sources.closers, milvusRecall)
	}
	sources.posts = append(sources.posts,
		model.NewContentPostRecallSource("content_hot", "popular content fallback", 2, contentService),
		model.NewContentPostRecallSource("content_fresh", "new content fallback", 1, contentService),
	)
	sources.users = []model.UserRecallSource{
		model.NewRedisUserRecallSource("mutual", "mutual connections", redisClient, identityUserKey(prefix, "mutual")),
		model.NewRedisUserRecallSource("interest", "shared interests", redisClient, identityUserKey(prefix, "interest")),
		model.NewRedisUserRecallSource("popular", "popular creator", redisClient, sceneUserKey(prefix, "popular")),
		model.NewRedisUserRecallSource("explore", "discover a creator", redisClient, sceneUserKey(prefix, "explore")),
		model.NewSocialUserRecallSource(userService),
	}
	return sources, nil
}

// Close 释放可选召回与推理客户端持有的连接，汇总所有关闭错误。
func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	var failures []error
	for _, closer := range s.closers {
		if closer != nil {
			failures = append(failures, closer.Close())
		}
	}
	return errors.Join(failures...)
}

// identityPostKey 生成按观看者身份分桶的帖子召回键；无身份时返回空键，表示该来源不适用。
func identityPostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.Identity == "" {
			return ""
		}
		return fmt.Sprintf("%s:recall:post:%s:%s:%s", prefix, source, req.Identity, req.Scene)
	}
}

// scenePostKey 生成只按场景分桶的帖子召回键（热门、探索等非个性化来源）。
func scenePostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		return fmt.Sprintf("%s:recall:post:%s:%s", prefix, source, req.Scene)
	}
}

// similarPostKey 生成按种子帖分桶的相似帖召回键；无种子时不适用。
func similarPostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.SeedPostID <= 0 {
			return ""
		}
		return fmt.Sprintf("%s:recall:post:%s:seed:%d:%s", prefix, source, req.SeedPostID, req.Scene)
	}
}

// identityUserKey 生成按观看者身份分桶的用户召回键。
func identityUserKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.Identity == "" {
			return ""
		}
		return fmt.Sprintf("%s:recall:user:%s:%s:%s", prefix, source, req.Identity, req.Scene)
	}
}

// sceneUserKey 生成只按场景分桶的用户召回键。
func sceneUserKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		return fmt.Sprintf("%s:recall:user:%s:%s", prefix, source, req.Scene)
	}
}

// randomSnapshotID 生成 144 位随机快照 ID，游标无法被猜测或枚举。
func randomSnapshotID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate recommendation snapshot id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
