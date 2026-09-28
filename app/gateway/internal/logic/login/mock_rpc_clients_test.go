package login

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/gateway/internal/svc"
	"esx/app/user/rpc/userservice"

	"github.com/stretchr/testify/mock"
)

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserResp)
	return v, args.Error(1)
}

func (m *MockUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...callopt.Option) (*userservice.BatchGetUsersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.BatchGetUsersResp)
	return v, args.Error(1)
}

func (m *MockUserService) SearchUsers(ctx context.Context, in *userservice.SearchUsersReq, opts ...callopt.Option) (*userservice.SearchUsersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SearchUsersResp)
	return v, args.Error(1)
}

func (m *MockUserService) UpdateProfile(ctx context.Context, in *userservice.UpdateProfileReq, opts ...callopt.Option) (*userservice.UpdateProfileResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UpdateProfileResp)
	return v, args.Error(1)
}

func (m *MockUserService) Follow(ctx context.Context, in *userservice.FollowReq, opts ...callopt.Option) (*userservice.FollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.FollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) Unfollow(ctx context.Context, in *userservice.UnfollowReq, opts ...callopt.Option) (*userservice.UnfollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UnfollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowersResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...callopt.Option) (*userservice.GetFollowingResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowingResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetUserTags(ctx context.Context, in *userservice.GetUserTagsReq, opts ...callopt.Option) (*userservice.GetUserTagsResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserTagsResp)
	return v, args.Error(1)
}

func (m *MockUserService) Register(ctx context.Context, in *userservice.RegisterReq, opts ...callopt.Option) (*userservice.RegisterResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.RegisterResp)
	return v, args.Error(1)
}

func (m *MockUserService) Login(ctx context.Context, in *userservice.LoginReq, opts ...callopt.Option) (*userservice.LoginResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.LoginResp)
	return v, args.Error(1)
}

func (m *MockUserService) RefreshToken(ctx context.Context, in *userservice.RefreshTokenReq, opts ...callopt.Option) (*userservice.RefreshTokenResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.RefreshTokenResp)
	return v, args.Error(1)
}

func (m *MockUserService) SendVerifyCode(ctx context.Context, in *userservice.SendVerifyCodeReq, opts ...callopt.Option) (*userservice.SendVerifyCodeResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SendVerifyCodeResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetPersonalizationPreference(ctx context.Context, in *userservice.GetPersonalizationPreferenceReq, opts ...callopt.Option) (*userservice.GetPersonalizationPreferenceResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetPersonalizationPreferenceResp)
	return v, args.Error(1)
}

func (m *MockUserService) SetPersonalizationPreference(ctx context.Context, in *userservice.SetPersonalizationPreferenceReq, opts ...callopt.Option) (*userservice.SetPersonalizationPreferenceResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SetPersonalizationPreferenceResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetAgentCapabilityConsent(ctx context.Context, in *userservice.GetAgentCapabilityConsentReq, opts ...callopt.Option) (*userservice.GetAgentCapabilityConsentResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetAgentCapabilityConsentResp)
	return v, args.Error(1)
}

func (m *MockUserService) SetAgentCapabilityConsent(ctx context.Context, in *userservice.SetAgentCapabilityConsentReq, opts ...callopt.Option) (*userservice.SetAgentCapabilityConsentResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SetAgentCapabilityConsentResp)
	return v, args.Error(1)
}

func newUnitSvcCtx(userSvc userservice.UserService) *svc.ServiceContext {
	return &svc.ServiceContext{
		UserService: userSvc,
	}
}
