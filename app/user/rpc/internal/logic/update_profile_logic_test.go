package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUpdateProfileLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.UpdateProfileReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
	}{
		{
			name: "成功更新资料",
			req:  &pb.UpdateProfileReq{UserId: 1, Nickname: "newNick", AvatarUrl: "http://a.jpg", Bio: "hello"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("UpdateUserDes", mock.Anything, int64(1), "newNick", "http://a.jpg", "hello").Return(nil).Once()
			},
		},
		{
			name: "DB 错误",
			req:  &pb.UpdateProfileReq{UserId: 1, Nickname: "nick"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("UpdateUserDes", mock.Anything, int64(1), "nick", "", "").Return(errors.New("db down")).Once()
			},
			wantErr: true,
			errCode: errx.SystemError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			logic := NewUpdateProfileLogic(context.Background(), svcCtx)

			resp, err := logic.UpdateProfile(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
			pm.AssertExpectations(t)
		})
	}
}
