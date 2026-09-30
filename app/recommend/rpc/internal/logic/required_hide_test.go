package logic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"esx/app/recommend/feedback"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/recommend"
	"esx/pkg/errx"
	redis "esx/pkg/redisstore"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"
)

type requiredHiddenRedis struct {
	*redis.Redis
	optedOut      bool
	failPositive  bool
	failNegative  bool
	hidden        map[string]string
	positiveReads int
	negativeReads int
}

func (r *requiredHiddenRedis) GetCtx(context.Context, string) (string, error) {
	if r.optedOut {
		return "1", nil
	}
	return "", nil
}
func (r *requiredHiddenRedis) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	if strings.HasSuffix(key, ":positive") {
		r.positiveReads++
		if r.failPositive {
			return nil, errors.New("optional profile unavailable")
		}
	}
	if key == "feature:v2:u:42:negative" {
		r.negativeReads++
		if r.failNegative {
			return nil, errors.New("required exclusions unavailable")
		}
		return r.hidden, nil
	}
	return map[string]string{}, nil
}
func (r *requiredHiddenRedis) LrangeCtx(context.Context, string, int, int) ([]string, error) {
	return nil, nil
}
func (r *requiredHiddenRedis) SmembersCtx(context.Context, string) ([]string, error) { return nil, nil }

type requiredPreferences struct{ err error }

func (p requiredPreferences) GetPersonalizationPreference(context.Context, *userservice.GetPersonalizationPreferenceReq, ...callopt.Option) (*userservice.GetPersonalizationPreferenceResp, error) {
	return &userservice.GetPersonalizationPreferenceResp{Enabled: true}, p.err
}
func requiredHideService(t *testing.T, r *requiredHiddenRedis, preferenceErr error) *svc.ServiceContext {
	t.Helper()
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	s.FeatureRepository = model.NewRedisFeatureRepository(r, "v2", requiredPreferences{err: preferenceErr})
	s.NegativeFeedback = feedback.NewHiddenPostReader(r, "v2")
	s.PostRecallSources = []model.PostRecallSource{fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
		return []model.PostCandidate{knownPost(99, 77, "a", 1), knownPost(100, 78, "b", 0.8), knownPost(101, 79, "c", 0.6), knownPost(102, 80, "d", 0.4)}, nil
	}}}
	return s
}
func requiredHideRequest() *pb.GetRecommendPostsReq {
	return &pb.GetRecommendPostsReq{UserId: 42, RequestId: "request", Scene: "home", SessionId: "session", PageSize: 2}
}
func TestFirstPageRequiresHiddenFeedbackIndependentlyOfProfile(t *testing.T) {
	for _, mode := range []string{"normal", "disabled", "preference_unknown", "profile_error", "profile_missing"} {
		t.Run(mode, func(t *testing.T) {
			r := &requiredHiddenRedis{hidden: map[string]string{"post:99": "1"}, optedOut: mode == "disabled", failPositive: mode == "profile_error"}
			var preferenceErr error
			if mode == "preference_unknown" {
				preferenceErr = errors.New("preference unavailable")
			}
			s := requiredHideService(t, r, preferenceErr)
			if mode == "profile_missing" {
				s.FeatureRepository = nil
			}
			response, err := NewGetRecommendPostsLogic(context.Background(), s).GetRecommendPosts(requiredHideRequest())
			require.NoError(t, err)
			require.NotEmpty(t, response.Posts)
			for _, post := range response.Posts {
				require.NotEqual(t, int64(99), post.PostId)
			}
			require.Positive(t, r.negativeReads)
			if mode == "disabled" || mode == "preference_unknown" || mode == "profile_missing" {
				require.Zero(t, r.positiveReads, "required filtering must not reactivate the optional profile")
			}
		})
	}
}
func TestFirstPageFailsClosedWithoutHiddenFeedback(t *testing.T) {
	for _, mode := range []string{"error", "missing_reader", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			r := &requiredHiddenRedis{hidden: map[string]string{"post:99": "1"}, failNegative: mode == "error"}
			if mode == "malformed" {
				r.hidden = map[string]string{"post:invalid": "1"}
			}
			s := requiredHideService(t, r, nil)
			if mode == "missing_reader" {
				s.NegativeFeedback = nil
			}
			response, err := NewGetRecommendPostsLogic(context.Background(), s).GetRecommendPosts(requiredHideRequest())
			require.True(t, errx.Is(err, errx.ServiceUnavailable), "%v", err)
			require.Nil(t, response)
		})
	}
}
func TestRuleOnlyCursorReadsOnlyRequiredHiddenFeedback(t *testing.T) {
	for _, mode := range []string{"disabled", "preference_unknown", "profile_error"} {
		t.Run(mode, func(t *testing.T) {
			r := &requiredHiddenRedis{hidden: map[string]string{"post:99": "1"}, optedOut: mode == "disabled", failPositive: mode == "profile_error"}
			var preferenceErr error
			if mode == "preference_unknown" {
				preferenceErr = errors.New("preference unavailable")
			}
			s := requiredHideService(t, r, preferenceErr)
			request := requiredHideRequest()
			request.PageSize = 1
			logic := NewGetRecommendPostsLogic(context.Background(), s)
			first, err := logic.GetRecommendPosts(request)
			require.NoError(t, err)
			require.NotEmpty(t, first.NextCursor)
			priorProfileReads := r.positiveReads
			r.hidden["post:101"] = "1"
			request.Cursor = first.NextCursor
			second, err := logic.GetRecommendPosts(request)
			require.NoError(t, err)
			require.Len(t, second.Posts, 1)
			require.Equal(t, int64(102), second.Posts[0].PostId)
			require.Equal(t, priorProfileReads, r.positiveReads, "paging must not read optional viewer features")
			r.failNegative = true
			failed, err := logic.GetRecommendPosts(request)
			require.Error(t, err)
			require.Nil(t, failed)
		})
	}
}
func TestAnonymousRecommendationDoesNotReadHiddenOrOptionalProfile(t *testing.T) {
	r := &requiredHiddenRedis{failNegative: true, failPositive: true}
	s := requiredHideService(t, r, nil)
	s.NegativeFeedback = nil
	request := requiredHideRequest()
	request.UserId = 0
	request.AnonymousId = "anonymous-session"
	response, err := NewGetRecommendPostsLogic(context.Background(), s).GetRecommendPosts(request)
	require.NoError(t, err)
	require.NotEmpty(t, response.Posts)
	require.Zero(t, r.positiveReads)
	require.Zero(t, r.negativeReads)
}
