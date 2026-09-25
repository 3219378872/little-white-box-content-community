package logic

import (
	"context"
	"errors"
	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingOptOutCache struct{ *memoryRedis }

func (r *failingOptOutCache) SetexCtx(context.Context, string, string, int) error {
	return errors.New("redis write failed")
}
func TestPersonalizationDisabledRemainsAuthoritativeAfterCacheWriteFailure(t *testing.T) {
	ctx := context.Background()
	store := &memoryPersonalizationStore{items: map[int64]*model.PersonalizationPreference{}}
	svcCtx := &svc.ServiceContext{Personalization: store, RedisClient: &failingOptOutCache{&memoryRedis{values: map[string]string{}}}}
	_, err := NewSetPersonalizationPreferenceLogic(ctx, svcCtx).SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 42, Enabled: false})
	require.NoError(t, err)
	preference, err := NewGetPersonalizationPreferenceLogic(ctx, svcCtx).GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 42})
	require.NoError(t, err)
	require.False(t, preference.Enabled)
	require.Positive(t, preference.OptedOutAt)
}
