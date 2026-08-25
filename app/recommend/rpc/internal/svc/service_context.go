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
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
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
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
	// content/user 的服务端挂了内部签名校验拦截器，出站必须同样签名；
	// 否则请求被 Unauthenticated 拒绝并经 errx 映射成 1006，推荐整体降级到规则。
	internalAuthInterceptor := interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)
	contentClient, err := zrpc.NewClient(c.ContentRpc,
		zrpc.WithUnaryClientInterceptor(bizErrInterceptor),
		zrpc.WithUnaryClientInterceptor(internalAuthInterceptor))
	if err != nil {
		return nil, fmt.Errorf("initialize content rpc client: %w", err)
	}
	userClient, err := zrpc.NewClient(c.UserRpc,
		zrpc.WithUnaryClientInterceptor(bizErrInterceptor),
		zrpc.WithUnaryClientInterceptor(internalAuthInterceptor))
	if err != nil {
		return nil, fmt.Errorf("initialize user rpc client: %w", err)
	}
	contentService := contentservice.NewContentService(contentClient)
	userService := userservice.NewUserService(userClient)
	prefix := c.RecallKeyPrefix + ":" + c.FeatureVersion
	postSources := []model.PostRecallSource{
		model.NewRedisPostRecallSource("follow", "from followed creators", redisClient, identityPostKey(prefix, "follow")),
		model.NewRedisPostRecallSource("hot", "popular now", redisClient, scenePostKey(prefix, "hot")),
		model.NewRedisPostRecallSource("explore", "explore something new", redisClient, scenePostKey(prefix, "explore")),
		model.NewRedisPostRecallSource("itemcf", "based on recent interests", redisClient, identityPostKey(prefix, "itemcf")),
	}
	similarSources := []model.PostRecallSource{
		model.NewRedisPostRecallSource("itemcf", "people also engaged with", redisClient, similarPostKey(prefix, "itemcf")),
	}
	closers := make([]io.Closer, 0, 1)
	if c.ElasticsearchRecall.Enabled {
		esRecall, err := model.NewElasticsearchPostRecallSource(
			c.ElasticsearchRecall.Addresses, c.ElasticsearchRecall.Index,
			c.ElasticsearchRecall.Username, c.ElasticsearchRecall.Password,
			c.FeatureVersion, redisClient, time.Duration(c.ElasticsearchRecall.TimeoutMs)*time.Millisecond,
		)
		if err != nil {
			return nil, err
		}
		postSources = append(postSources, esRecall)
		similarSources = append(similarSources, esRecall)
	}
	if c.MilvusRecall.Enabled {
		milvusRecall := model.NewMilvusPostRecallSource(
			c.MilvusRecall.Address, c.MilvusRecall.Collection,
			c.MilvusRecall.Username, c.MilvusRecall.Password, c.MilvusRecall.Database,
			c.FeatureVersion, c.MilvusRecall.NProbe, redisClient,
			time.Duration(c.MilvusRecall.TimeoutMs)*time.Millisecond,
		)
		postSources = append(postSources, milvusRecall)
		similarSources = append(similarSources, milvusRecall)
		closers = append(closers, milvusRecall)
	}
	postSources = append(postSources,
		model.NewContentPostRecallSource("content_hot", "popular content fallback", 2, contentService),
		model.NewContentPostRecallSource("content_fresh", "new content fallback", 1, contentService),
	)
	userSources := []model.UserRecallSource{
		model.NewRedisUserRecallSource("mutual", "mutual connections", redisClient, identityUserKey(prefix, "mutual")),
		model.NewRedisUserRecallSource("interest", "shared interests", redisClient, identityUserKey(prefix, "interest")),
		model.NewRedisUserRecallSource("popular", "popular creator", redisClient, sceneUserKey(prefix, "popular")),
		model.NewRedisUserRecallSource("explore", "discover a creator", redisClient, sceneUserKey(prefix, "explore")),
		model.NewSocialUserRecallSource(userService),
	}
	serviceContext := &ServiceContext{
		Config:             c,
		ContentService:     contentService,
		UserService:        userService,
		PostRecallSources:  postSources,
		SimilarPostSources: similarSources,
		UserRecallSources:  userSources,
		FeatureRepository:  model.NewRedisFeatureRepository(redisClient, c.FeatureVersion),
		SnapshotStore:      model.NewRedisSnapshotStore(redisClient, prefix),
		CursorCodec:        cursorCodec,
		Now:                now,
		NewSnapshotID:      randomSnapshotID,
		closers:            closers,
	}
	if c.OnlineInfer.Enabled {
		client, err := zrpc.NewClient(c.OnlineInfer.Rpc)
		if err != nil {
			return nil, fmt.Errorf("initialize online inference client: %w", err)
		}
		serviceContext.InferenceRanker = model.NewGRPCInferenceRanker(inferencepb.NewOnlineInferServiceClient(client.Conn()))
	}
	return serviceContext, nil
}

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

func identityPostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.Identity == "" {
			return ""
		}
		return fmt.Sprintf("%s:recall:post:%s:%s:%s", prefix, source, req.Identity, req.Scene)
	}
}

func scenePostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		return fmt.Sprintf("%s:recall:post:%s:%s", prefix, source, req.Scene)
	}
}

func similarPostKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.SeedPostID <= 0 {
			return ""
		}
		return fmt.Sprintf("%s:recall:post:%s:seed:%d:%s", prefix, source, req.SeedPostID, req.Scene)
	}
}

func identityUserKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		if req.Identity == "" {
			return ""
		}
		return fmt.Sprintf("%s:recall:user:%s:%s:%s", prefix, source, req.Identity, req.Scene)
	}
}

func sceneUserKey(prefix, source string) func(model.RecallRequest) string {
	return func(req model.RecallRequest) string {
		return fmt.Sprintf("%s:recall:user:%s:%s", prefix, source, req.Scene)
	}
}

func randomSnapshotID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate recommendation snapshot id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
