package store

import (
	"context"
	"errors"
	"esx/app/user/rpc/userservice"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type preferenceReader struct {
	enabled     bool
	err         error
	calls       int
	nilResponse bool
}

func (r *preferenceReader) GetPersonalizationPreference(_ context.Context, req *userservice.GetPersonalizationPreferenceReq, _ ...grpc.CallOption) (*userservice.GetPersonalizationPreferenceResp, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if r.nilResponse {
		return nil, nil
	}
	return &userservice.GetPersonalizationPreferenceResp{Enabled: r.enabled}, nil
}
func enabledPreferences() *preferenceReader { return &preferenceReader{enabled: true} }

func TestBehaviorRequiresDurableConsentWithoutMarker(t *testing.T) {
	for _, tc := range []struct {
		name          string
		enabled       bool
		cacheErr      error
		preferenceErr error
		noReader      bool
		nilResponse   bool
	}{
		{name: "missing marker and opted out"},
		{name: "expired marker and enabled", enabled: true},
		{name: "cache unavailable and opted out", cacheErr: errors.New("cache down")},
		{name: "authority unavailable", preferenceErr: errors.New("user rpc down")},
		{name: "unconfigured authority", noReader: true},
		{name: "empty authority response", nilResponse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			redis := &fakeGetterEvaler{fakeEvaler: &fakeEvaler{}, getErrs: map[string]error{"personalization:optout:42": tc.cacheErr}}
			reader := &preferenceReader{enabled: tc.enabled, err: tc.preferenceErr, nilResponse: tc.nilResponse}
			var dep PersonalizationPreferenceReader = reader
			if tc.noReader {
				dep = nil
			}
			store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600, dep)
			err := store.Record(context.Background(), featureBehavior())
			if tc.preferenceErr != nil || tc.noReader || tc.nilResponse {
				require.Error(t, err)
				require.Empty(t, redis.keys)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, reader.calls)
			if tc.enabled {
				require.Contains(t, redis.keys, "feature:v2:dedup:1")
			} else {
				require.NotContains(t, redis.keys, "feature:v2:dedup:1")
				require.Contains(t, redis.keys, "feature:v2:u:42:recent")
			}
		})
	}
}

type discoveredProfileRedis struct {
	fakePurgeRedis
	calls map[string]int
}

func (r *discoveredProfileRedis) KeysCtx(_ context.Context, pattern string) ([]string, error) {
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[pattern]++
	var out []string
	for _, key := range r.keys {
		matched, _ := path.Match(pattern, key)
		if matched {
			out = append(out, key)
		}
	}
	return out, nil
}
func TestCleanupFindsDurableOptOutWithoutMarker(t *testing.T) {
	redis := &discoveredProfileRedis{fakePurgeRedis: fakePurgeRedis{keys: []string{"feature:v2:u:42:recent", "recommend:v2:recall:post:follow:u:42:home", "recommend:v2:recall:post:follow:u:420:home"}}}
	reader := &preferenceReader{}
	store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600, reader)
	count, err := store.PurgeOptedOutFeatures(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count)
	for _, keys := range redis.purges {
		if keys[0] == "feature:v2:u:42:recent" {
			require.Contains(t, keys, "recommend:v2:recall:post:follow:u:42:home")
			require.NotContains(t, keys, "recommend:v2:recall:post:follow:u:420:home")
		}
	}
}
func TestCleanupAuthorityFailureDoesNotDiscardKnownOptOut(t *testing.T) {
	redis := &discoveredProfileRedis{fakePurgeRedis: fakePurgeRedis{keys: []string{"personalization:optout:7", "feature:v2:u:42:recent"}}}
	store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600, &preferenceReader{err: errors.New("authority unavailable")})
	count, err := store.PurgeOptedOutFeatures(context.Background())
	require.ErrorContains(t, err, "authority unavailable")
	require.Equal(t, 1, count)
	require.Len(t, redis.purges, 1)
	require.Equal(t, "feature:v2:u:7:recent", redis.purges[0][0])
}
