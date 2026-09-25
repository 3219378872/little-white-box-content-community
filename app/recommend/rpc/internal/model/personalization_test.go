package model

import (
	"context"
	"errors"
	"esx/app/user/rpc/userservice"
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

func (r *preferenceReader) GetPersonalizationPreference(context.Context, *userservice.GetPersonalizationPreferenceReq, ...grpc.CallOption) (*userservice.GetPersonalizationPreferenceResp, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if r.nilResponse {
		return nil, nil
	}
	return &userservice.GetPersonalizationPreferenceResp{Enabled: r.enabled}, nil
}

type failingPreferenceRedis struct {
	*fakeRedisClient
	err error
}

func (r *failingPreferenceRedis) GetCtx(ctx context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.fakeRedisClient.GetCtx(ctx, key)
}
func TestRecommendationRequiresDurableConsentWithoutMarker(t *testing.T) {
	for _, tc := range []struct {
		name         string
		enabled      bool
		cacheErr     error
		authorityErr error
		nilResponse  bool
		noReader     bool
	}{
		{name: "expired opt-out marker"}, {name: "new user consent", enabled: true},
		{name: "cache lost", cacheErr: errors.New("redis down")},
		{name: "unknown authority", authorityErr: errors.New("user unavailable")},
		{name: "empty response", nilResponse: true}, {name: "unconfigured authority", noReader: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &preferenceReader{enabled: tc.enabled, err: tc.authorityErr, nilResponse: tc.nilResponse}
			var dep PersonalizationPreferenceReader = reader
			if tc.noReader {
				dep = nil
			}
			repo := NewRedisFeatureRepository(&failingPreferenceRedis{fakeRedisClient: &fakeRedisClient{}, err: tc.cacheErr}, "v2", dep)
			out, err := repo.IsPersonalizationOptedOut(context.Background(), 42)
			if tc.authorityErr != nil || tc.nilResponse || tc.noReader {
				require.Error(t, err)
				require.True(t, out)
			} else {
				require.NoError(t, err)
				require.Equal(t, !tc.enabled, out)
				require.Equal(t, 1, reader.calls)
			}
		})
	}
}
