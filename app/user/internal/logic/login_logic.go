package logic

import (
	"context"
	"fmt"
	"jwtx"

	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 登录
func (l *LoginLogic) Login(in *pb.LoginReq) (*pb.LoginResp, error) {
	userId := 666
	username := "张三"
	return &pb.LoginResp{
		UserId: int64(userId),
		Token: fmt.Sprintf("%v", jwtx.Claims{
			UserId:           int64(userId),
			Username:         username,
			RegisteredClaims: jwt.RegisteredClaims{},
		}),
		User: &pb.UserInfo{
			Id:             0,
			Username:       username,
			Nickname:       username + "666",
			AvatarUrl:      "",
			Bio:            "",
			Level:          0,
			FollowerCount:  0,
			FollowingCount: 0,
			PostCount:      0,
			LikeCount:      0,
			CreatedAt:      0,
		},
	}, nil
}
