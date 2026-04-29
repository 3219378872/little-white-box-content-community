package login

import (
	"context"

	"gateway/internal/svc"
	"user/userservice"

	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserResp)
	return v, args.Error(1)
}

func (m *MockUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.BatchGetUsersResp)
	return v, args.Error(1)
}

func (m *MockUserService) UpdateProfile(ctx context.Context, in *userservice.UpdateProfileReq, opts ...grpc.CallOption) (*userservice.UpdateProfileResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UpdateProfileResp)
	return v, args.Error(1)
}

func (m *MockUserService) Follow(ctx context.Context, in *userservice.FollowReq, opts ...grpc.CallOption) (*userservice.FollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.FollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) Unfollow(ctx context.Context, in *userservice.UnfollowReq, opts ...grpc.CallOption) (*userservice.UnfollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UnfollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowersResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowingResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetUserTags(ctx context.Context, in *userservice.GetUserTagsReq, opts ...grpc.CallOption) (*userservice.GetUserTagsResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserTagsResp)
	return v, args.Error(1)
}

func (m *MockUserService) Register(ctx context.Context, in *userservice.RegisterReq, opts ...grpc.CallOption) (*userservice.RegisterResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.RegisterResp)
	return v, args.Error(1)
}

func (m *MockUserService) Login(ctx context.Context, in *userservice.LoginReq, opts ...grpc.CallOption) (*userservice.LoginResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.LoginResp)
	return v, args.Error(1)
}

func (m *MockUserService) SendVerifyCode(ctx context.Context, in *userservice.SendVerifyCodeReq, opts ...grpc.CallOption) (*userservice.SendVerifyCodeResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SendVerifyCodeResp)
	return v, args.Error(1)
}

func newUnitSvcCtx(userSvc userservice.UserService) *svc.ServiceContext {
	return &svc.ServiceContext{
		UserService: userSvc,
	}
}
