package logic

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"sync"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type memoryPersonalizationStore struct {
	items map[int64]*model.PersonalizationPreference
	err   error
}

func (s *memoryPersonalizationStore) Get(_ context.Context, userID int64) (*model.PersonalizationPreference, error) {
	if s.err != nil {
		return nil, s.err
	}
	pref, ok := s.items[userID]
	if !ok {
		return nil, model.ErrPersonalizationPreferenceNotFound
	}
	return pref, nil
}

func (s *memoryPersonalizationStore) Upsert(_ context.Context, pref *model.PersonalizationPreference) error {
	if s.err != nil {
		return s.err
	}
	s.items[pref.UserID] = pref
	return nil
}

type memoryRedis struct {
	*redis.Redis
	mu     sync.Mutex
	values map[string]string
}

func (r *memoryRedis) SetexCtx(_ context.Context, key, value string, seconds int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *memoryRedis) DelCtx(_ context.Context, keys ...string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range keys {
		delete(r.values, key)
	}
	return len(keys), nil
}

func (r *memoryRedis) GetCtx(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.values[key], nil
}

func (r *memoryRedis) EvalCtx(_ context.Context, _ string, keys []string, args ...any) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(keys) != 1 || len(args) != 1 {
		return nil, errors.New("memory redis: invalid eval arguments")
	}
	want, ok := args[0].(string)
	if !ok {
		return nil, errors.New("memory redis: invalid eval owner")
	}
	current, exists := r.values[keys[0]]
	if !exists {
		return int64(0), nil
	}
	if current != want {
		return int64(-1), nil
	}
	delete(r.values, keys[0])
	return int64(1), nil
}

func (r *memoryRedis) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.values[key]; exists {
		return false, nil
	}
	r.values[key] = value
	return true, nil
}

func (r *memoryRedis) IncrCtx(_ context.Context, key string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.values[key]
	var current int64
	if value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			current = parsed
		}
	}
	current++
	r.values[key] = strconv.FormatInt(current, 10)
	return current, nil
}

func (r *memoryRedis) ExpireCtx(_ context.Context, _ string, _ int) error {
	return nil
}

func TestGetPersonalizationPreference(t *testing.T) {
	t.Run("默认开启", func(t *testing.T) {
		store := &memoryPersonalizationStore{items: map[int64]*model.PersonalizationPreference{}}
		svcCtx := &svc.ServiceContext{Personalization: store}
		resp, err := NewGetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 1})
		require.NoError(t, err)
		assert.True(t, resp.Enabled)
	})

	t.Run("已关闭返回关闭时间", func(t *testing.T) {
		store := &memoryPersonalizationStore{items: map[int64]*model.PersonalizationPreference{
			1: {UserID: 1, Enabled: false, OptedOutAt: sql.NullInt64{Int64: 12345, Valid: true}},
		}}
		svcCtx := &svc.ServiceContext{Personalization: store}
		resp, err := NewGetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 1})
		require.NoError(t, err)
		assert.False(t, resp.Enabled)
		assert.Equal(t, int64(12345), resp.OptedOutAt)
	})

	t.Run("参数错误", func(t *testing.T) {
		svcCtx := &svc.ServiceContext{Personalization: &memoryPersonalizationStore{}}
		_, err := NewGetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 0})
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.ParamError))
	})
}

func TestSetPersonalizationPreference(t *testing.T) {
	t.Run("关闭时写入标记并记录时间", func(t *testing.T) {
		store := &memoryPersonalizationStore{items: map[int64]*model.PersonalizationPreference{}}
		memRedis := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{Personalization: store, RedisClient: memRedis}

		_, err := NewSetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 7, Enabled: false})
		require.NoError(t, err)
		pref := store.items[7]
		require.NotNil(t, pref)
		assert.False(t, pref.Enabled)
		assert.NotZero(t, pref.OptedOutAt.Int64)
		assert.Equal(t, "1", memRedis.values["personalization:optout:7"])
	})

	t.Run("重新开启时清除标记", func(t *testing.T) {
		store := &memoryPersonalizationStore{items: map[int64]*model.PersonalizationPreference{
			7: {UserID: 7, Enabled: false, OptedOutAt: sql.NullInt64{Int64: 123, Valid: true}},
		}}
		memRedis := &memoryRedis{values: map[string]string{"personalization:optout:7": "1"}}
		svcCtx := &svc.ServiceContext{Personalization: store, RedisClient: memRedis}

		_, err := NewSetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 7, Enabled: true})
		require.NoError(t, err)
		assert.NotContains(t, memRedis.values, "personalization:optout:7")
	})

	t.Run("存储失败返回系统错误", func(t *testing.T) {
		svcCtx := &svc.ServiceContext{Personalization: &memoryPersonalizationStore{
			items: map[int64]*model.PersonalizationPreference{}, err: errors.New("db down"),
		}}
		_, err := NewSetPersonalizationPreferenceLogic(context.Background(), svcCtx).
			SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 7, Enabled: false})
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.SystemError))
	})
}
