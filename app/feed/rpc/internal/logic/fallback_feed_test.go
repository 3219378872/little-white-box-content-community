package logic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type memoryFallbackStates struct {
	values  map[string][]byte
	saveErr error
	loadErr error
	ttl     int
}

func newMemoryFallbackStates() *memoryFallbackStates {
	return &memoryFallbackStates{values: make(map[string][]byte)}
}
func (s *memoryFallbackStates) Save(_ context.Context, id string, state model.FallbackState, ttl int) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	s.values[id], s.ttl = raw, ttl
	return nil
}
func (s *memoryFallbackStates) Load(_ context.Context, id string) (model.FallbackState, error) {
	if s.loadErr != nil {
		return model.FallbackState{}, s.loadErr
	}
	raw, ok := s.values[id]
	if !ok {
		return model.FallbackState{}, model.ErrFallbackStateMissing
	}
	var state model.FallbackState
	err := json.Unmarshal(raw, &state)
	return state, err
}

type fakeNegativeFeedback struct {
	hidden map[int64]struct{}
	err    error
}

func (f *fakeNegativeFeedback) HiddenPosts(context.Context, int64) (map[int64]struct{}, error) {
	return f.hidden, f.err
}

func TestRecommendFeedPreservesEmptyAndInvalidCursorResults(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *recommendservice.GetRecommendPostsResp
		err      error
		cursor   string
	}{
		{"all candidates hidden", &recommendservice.GetRecommendPostsResp{RequestId: "req"}, nil, ""},
		{"invalid cursor", nil, errx.NewWithCode(errx.ParamError), "tampered"},
		{"cursor service outage", nil, errx.NewWithCode(errx.ServiceUnavailable), "valid-but-unverifiable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := new(mockRecommendService)
			rec.On("GetRecommendPosts", mock.Anything, mock.Anything).Return(test.response, test.err).Once()
			content := new(mockContentService)
			l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{RecommendService: rec, ContentService: content})
			response, err := l.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 7, PageSize: 2, RequestId: "req", Cursor: test.cursor})
			if test.err == nil {
				require.NoError(t, err)
				assert.Empty(t, response.Items)
			} else {
				require.Error(t, err)
				assert.Nil(t, response)
				assert.Equal(t, errx.GetCode(test.err), errx.GetCode(err))
			}
			content.AssertNotCalled(t, "GetPostList", mock.Anything, mock.Anything)
		})
	}
}

func TestFallbackRetainsPendingRowsAndNeverRestartsExhaustedSource(t *testing.T) {
	content := new(mockContentService)
	content.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{PageSize: 2, SortBy: 3}).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}, {Id: 2, Status: 1}}, NextCursor: "hot-next"}, nil).Once()
	content.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{PageSize: 2, SortBy: 3, Cursor: "hot-next"}).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 5, Status: 1}, {Id: 6, Status: 1}}}, nil).Once()
	content.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{PageSize: 2, SortBy: 1}).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 3, Status: 1}, {Id: 4, Status: 1}}}, nil).Once()
	content.On("GetPostsByIds", mock.Anything, mock.Anything).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}, {Id: 2, Status: 1}, {Id: 3, Status: 1}, {Id: 4, Status: 1}, {Id: 5, Status: 1}, {Id: 6, Status: 1}}}, nil).Times(3)
	states := newMemoryFallbackStates()
	now := time.Unix(1800000000, 0)
	l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{Config: config.Config{CursorSecret: "secret"}, ContentService: content, FallbackStates: states, Now: func() time.Time { return now }})
	request := &pb.GetRecommendFeedReq{AnonymousId: "anon", RequestId: "req", PageSize: 2}
	var returned []int64
	for page := 0; page < 3; page++ {
		response, err := l.GetRecommendFeed(request)
		require.NoError(t, err)
		for _, item := range response.Items {
			returned = append(returned, item.PostId)
			assert.Equal(t, int32(len(returned)), item.Position)
		}
		assert.Equal(t, page < 2, response.HasMore)
		request.Cursor = response.NextCursor
		now = now.Add(time.Minute)
	}
	assert.Equal(t, []int64{1, 3, 2, 4, 5, 6}, returned)
	assert.Equal(t, 540, states.ttl)
	content.AssertExpectations(t)
}

func TestFallbackFiltersHiddenAndClosesOnFilterFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "hidden excluded", true: "filter failure"}[fail], func(t *testing.T) {
			feedback := &fakeNegativeFeedback{hidden: map[int64]struct{}{44: {}}}
			content := new(mockContentService)
			if fail {
				feedback.err = errors.New("redis unavailable")
			} else {
				content.On("GetPostList", mock.Anything, mock.Anything).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 44, Status: 1}, {Id: 45, Status: 1}}}, nil).Twice()
				content.On("GetPostsByIds", mock.Anything, mock.Anything).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 44, Status: 1}, {Id: 45, Status: 1}}}, nil).Once()
			}
			l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: content, NegativeFeedback: feedback})
			response, err := l.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 7, PageSize: 2, RequestId: "req"})
			if fail {
				assert.Nil(t, response)
				assert.True(t, errx.Is(err, errx.ServiceUnavailable))
				return
			}
			require.NoError(t, err)
			require.Len(t, response.Items, 1)
			assert.Equal(t, int64(45), response.Items[0].PostId)
			assert.False(t, response.HasMore)
		})
	}
}

func TestFallbackStateSaveFailureDoesNotReturnSuccess(t *testing.T) {
	content := new(mockContentService)
	content.On("GetPostList", mock.Anything, mock.Anything).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}}, NextCursor: "more"}, nil).Twice()
	content.On("GetPostsByIds", mock.Anything, mock.Anything).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}}}, nil).Once()
	states := newMemoryFallbackStates()
	states.saveErr = errors.New("redis unavailable")
	l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{Config: config.Config{CursorSecret: "secret"}, ContentService: content, FallbackStates: states})
	response, err := l.GetRecommendFeed(&pb.GetRecommendFeedReq{AnonymousId: "anon", PageSize: 2, RequestId: "req"})
	assert.Nil(t, response)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
}

func TestFallbackLastRemainingSourceFailureIsNotEndOfFeed(t *testing.T) {
	content := new(mockContentService)
	content.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{PageSize: 2, SortBy: 3, Cursor: "more"}).Return(nil, errors.New("content unavailable")).Once()
	l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: content})
	state := model.FallbackState{Sources: []model.FallbackSource{{Name: "popular", Cursor: "more"}, {Name: "latest", Exhausted: true}}}
	response, err := l.fallbackPage(&pb.GetRecommendFeedReq{AnonymousId: "anon", PageSize: 2}, state)
	assert.Nil(t, response)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
	assert.False(t, state.Sources[0].Exhausted)
	content.AssertExpectations(t)
}

func TestFallbackFailedSourceRemainsAvailableOnContinuation(t *testing.T) {
	content := new(mockContentService)
	popularRequest := &contentservice.GetPostListReq{PageSize: 2, SortBy: 3}
	content.On("GetPostList", mock.Anything, popularRequest).Return(nil, errors.New("temporarily unavailable")).Once()
	content.On("GetPostList", mock.Anything, popularRequest).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 3, Status: 1}, {Id: 4, Status: 1}}}, nil).Once()
	content.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{PageSize: 2, SortBy: 1}).Return(&contentservice.GetPostListResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}, {Id: 2, Status: 1}}}, nil).Once()
	content.On("GetPostsByIds", mock.Anything, mock.Anything).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1}, {Id: 2, Status: 1}, {Id: 3, Status: 1}, {Id: 4, Status: 1}}}, nil).Twice()
	l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{Config: config.Config{CursorSecret: "secret"}, ContentService: content, FallbackStates: newMemoryFallbackStates()})
	request := &pb.GetRecommendFeedReq{AnonymousId: "anon", PageSize: 2, RequestId: "req"}
	first, err := l.GetRecommendFeed(request)
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	assert.Equal(t, int64(1), first.Items[0].PostId)
	assert.Equal(t, int64(2), first.Items[1].PostId)
	assert.True(t, first.HasMore)
	request.Cursor = first.NextCursor
	second, err := l.GetRecommendFeed(request)
	require.NoError(t, err)
	require.Len(t, second.Items, 2)
	assert.Equal(t, int64(3), second.Items[0].PostId)
	assert.Equal(t, int64(4), second.Items[1].PostId)
	assert.False(t, second.HasMore)
	content.AssertExpectations(t)
}

func TestFallbackFollowSourceRejectsCursorWithoutProgress(t *testing.T) {
	inbox := new(mockInboxModel)
	outbox := new(mockOutboxModel)
	users := new(mockUserService)
	content := new(mockContentService)
	users.On("GetFollowing", mock.Anything, mock.Anything).Return(&userservice.GetFollowingResp{Users: followingUsers(8), Total: 1}, nil).Once()
	inbox.On("FindByUserBefore", mock.Anything, int64(7), int64(1000), int64(10), int64(2)).Return([]*model.FeedInbox{{AuthorId: 8, PostId: 10, CreatedAt: 1000}, {AuthorId: 8, PostId: 9, CreatedAt: 999}}, nil).Once()
	outbox.On("FindByAuthorsBefore", mock.Anything, []int64{8}, int64(1000), int64(10), int64(2)).Return(nil, nil).Once()
	content.On("GetPostsByIds", mock.Anything, mock.Anything).Return(&contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 10, AuthorId: 8, Status: 1, CreatedAt: 1000}, {Id: 9, AuthorId: 8, Status: 1, CreatedAt: 999}}}, nil).Once()
	l := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: users, ContentService: content})
	source := model.FallbackSource{Name: "follow", CursorCreatedAt: 1000, CursorPostID: 10}
	err := l.refillFallbackSource(&pb.GetRecommendFeedReq{UserId: 7, PageSize: 1}, &source)
	require.ErrorContains(t, err, "cursor did not advance")
	assert.Equal(t, model.FallbackSource{Name: "follow", CursorCreatedAt: 1000, CursorPostID: 10}, source)
	inbox.AssertExpectations(t)
	outbox.AssertExpectations(t)
	users.AssertExpectations(t)
	content.AssertExpectations(t)
}
