package logic

import (
	"context"
	"database/sql"
	"esx/pkg/outboxx"
	"esx/pkg/util"
	"time"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"

	"github.com/stretchr/testify/mock"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func init() {
	_ = util.InitSnowflake(1, 1)
}

// ── Mock UserProfileModel ─────────────────────────────────────────────────────

type MockUserProfileModel struct{ mock.Mock }

func (m *MockUserProfileModel) Insert(ctx context.Context, data *model.UserProfile) (sql.Result, error) {
	args := m.Called(ctx, data)
	r, _ := args.Get(0).(sql.Result)
	return r, args.Error(1)
}

func (m *MockUserProfileModel) FindOne(ctx context.Context, id int64) (*model.UserProfile, error) {
	args := m.Called(ctx, id)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) FindByIDs(ctx context.Context, ids []int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, ids)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) SearchPublic(ctx context.Context, keyword string, offset, limit int64) ([]*model.UserProfile, int64, error) {
	args := m.Called(ctx, keyword, offset, limit)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Get(1).(int64), args.Error(2)
}

func (m *MockUserProfileModel) FindOneByPhone(ctx context.Context, phone sql.NullString) (*model.UserProfile, error) {
	args := m.Called(ctx, phone)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) FindOneByUsername(ctx context.Context, username string) (*model.UserProfile, error) {
	args := m.Called(ctx, username)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) Update(ctx context.Context, data *model.UserProfile) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockUserProfileModel) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockUserProfileModel) UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error {
	args := m.Called(ctx, userId, nickname, avatarUrl, bio)
	return args.Error(0)
}

func (m *MockUserProfileModel) FindOneByIdForUpdate(ctx context.Context, session sqlx.Session, id int64) (*model.UserProfile, error) {
	args := m.Called(ctx, session, id)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

// ── Mock UserFollowStore ──────────────────────────────────────────────────────

type MockUserFollowStore struct{ mock.Mock }

func (m *MockUserFollowStore) Follow(ctx context.Context, userID, targetUserID int64) error {
	return m.Called(ctx, userID, targetUserID).Error(0)
}

func (m *MockUserFollowStore) Unfollow(ctx context.Context, userID, targetUserID int64) error {
	return m.Called(ctx, userID, targetUserID).Error(0)
}

func (m *MockUserFollowStore) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserFollowStore) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserFollowStore) CountFollowers(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockUserFollowStore) CountFollowing(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// ── SvcCtx Builder ────────────────────────────────────────────────────────────

func newUnitSvcCtx(
	profileModel svc.UserProfileStore,
	followStore svc.UserFollowStore,
) *svc.ServiceContext {
	return &svc.ServiceContext{
		UserProfileModel:   profileModel,
		UserFollowModel:    followStore,
		UserFollowCommands: legacyUserFollowCommands{store: followStore},
	}
}

type legacyUserFollowCommands struct {
	store svc.UserFollowStore
}

func (c legacyUserFollowCommands) Follow(
	ctx context.Context,
	userID, targetUserID int64,
	_ outboxx.Event,
) error {
	return c.store.Follow(ctx, userID, targetUserID)
}

func (c legacyUserFollowCommands) Unfollow(
	ctx context.Context,
	userID, targetUserID int64,
	_ outboxx.Event,
) error {
	return c.store.Unfollow(ctx, userID, targetUserID)
}

// ── Shared Helpers ────────────────────────────────────────────────────────────

func sampleUser(id int64, username string) *model.UserProfile {
	return &model.UserProfile{
		Id:             id,
		Username:       username,
		Password:       "$2a$10$dummyhashdummyhashdummyhashAB",
		CreatedAt:      time.Unix(1710000000, 0),
		FollowerCount:  3,
		FollowingCount: 5,
	}
}
