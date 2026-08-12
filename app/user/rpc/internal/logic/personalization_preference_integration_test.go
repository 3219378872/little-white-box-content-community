//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalizationPreferenceRoundTrip(t *testing.T) {
	ctx := context.Background()

	// 默认开启
	get, err := NewGetPersonalizationPreferenceLogic(ctx, testSvcCtx).
		GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 7001})
	require.NoError(t, err)
	assert.True(t, get.Enabled)

	// 关闭：记录关闭时间 + Redis 标记
	_, err = NewSetPersonalizationPreferenceLogic(ctx, testSvcCtx).
		SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 7001, Enabled: false})
	require.NoError(t, err)

	get, err = NewGetPersonalizationPreferenceLogic(ctx, testSvcCtx).
		GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 7001})
	require.NoError(t, err)
	assert.False(t, get.Enabled)
	assert.NotZero(t, get.OptedOutAt)

	marker, err := testSvcCtx.RedisClient.GetCtx(ctx, "personalization:optout:7001")
	require.NoError(t, err)
	assert.Equal(t, "1", marker)

	// 重新开启：清除标记
	_, err = NewSetPersonalizationPreferenceLogic(ctx, testSvcCtx).
		SetPersonalizationPreference(&pb.SetPersonalizationPreferenceReq{UserId: 7001, Enabled: true})
	require.NoError(t, err)

	get, err = NewGetPersonalizationPreferenceLogic(ctx, testSvcCtx).
		GetPersonalizationPreference(&pb.GetPersonalizationPreferenceReq{UserId: 7001})
	require.NoError(t, err)
	assert.True(t, get.Enabled)
}
