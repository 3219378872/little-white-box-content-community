package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"esx/app/recommend/rpc/internal/model"
	pb "esx/kitex_gen/recommend"
	"esx/pkg/errx"
)

// requireUserMessage 断言错误保持 ParamError 码，且经网关透传给用户的文案是指定中文。
func requireUserMessage(t *testing.T, err error, want string) {
	t.Helper()
	var biz *errx.BizError
	require.True(t, errors.As(err, &biz), "%v", err)
	require.Equal(t, errx.ParamError, biz.Code)
	require.Equal(t, want, biz.Message)
}

// firstRecommendPage 返回一个三条候选、每页两条的首屏，供续页错误路径使用。
func firstRecommendPage(t *testing.T) (*GetRecommendPostsLogic, *pb.GetRecommendPostsReq, *fakeFeatureRepository, *memorySnapshotStore) {
	t.Helper()
	s := newTestServiceContext(t, time.Unix(1800000000, 0))
	repo := &fakeFeatureRepository{}
	s.FeatureRepository = repo
	s.PostRecallSources = []model.PostRecallSource{fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
		return []model.PostCandidate{knownPost(1, 11, "a", 0.9), knownPost(2, 12, "b", 0.8), knownPost(3, 13, "c", 0.7)}, nil
	}}}
	l := NewGetRecommendPostsLogic(context.Background(), s)
	req := &pb.GetRecommendPostsReq{UserId: 1, RequestId: "request", Scene: "home", PageSize: 2}
	first, err := l.GetRecommendPosts(req)
	require.NoError(t, err)
	require.NotEmpty(t, first.NextCursor)
	req.Cursor = first.NextCursor
	return l, req, repo, s.SnapshotStore.(*memorySnapshotStore)
}

func TestRecommendCursorErrorsUseChineseUserMessages(t *testing.T) {
	t.Run("tampered cursor", func(t *testing.T) {
		l, req, _, _ := firstRecommendPage(t)
		req.Cursor = "tampered.cursor"
		_, err := l.GetRecommendPosts(req)
		requireUserMessage(t, err, "推荐列表已失效，请刷新后重试")
	})
	t.Run("snapshot evicted", func(t *testing.T) {
		l, req, _, store := firstRecommendPage(t)
		clear(store.items)
		_, err := l.GetRecommendPosts(req)
		requireUserMessage(t, err, "推荐列表已失效，请刷新后重试")
	})
	t.Run("offset beyond snapshot", func(t *testing.T) {
		l, req, _, store := firstRecommendPage(t)
		for id, snapshot := range store.items {
			snapshot.Posts = snapshot.Posts[:1]
			store.items[id] = snapshot
		}
		_, err := l.GetRecommendPosts(req)
		requireUserMessage(t, err, "推荐列表已失效，请刷新后重试")
	})
	t.Run("personalization changed", func(t *testing.T) {
		l, req, repo, _ := firstRecommendPage(t)
		repo.optedOut = true
		_, err := l.GetRecommendPosts(req)
		requireUserMessage(t, err, "个性化推荐设置已变更，请刷新推荐列表")
	})
}
