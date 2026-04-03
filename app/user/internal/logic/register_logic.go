package logic

import (
	"context"
	"database/sql"
	"errx"
	"jwtx"
	"user/internal/model"
	"util"

	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 注册
func (l *RegisterLogic) Register(in *pb.RegisterReq) (*pb.RegisterResp, error) {
	user, err := newUser(in)
	if err != nil {
		return nil, err
	}
	_, err = l.svcCtx.UserProfileModel.Insert(l.ctx, user)
	if err != nil {
		return nil, errx.NewWithCode(errx.USER_ALREADY_EXIST)
	}
	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		return nil, err
	}
	return &pb.RegisterResp{
		UserId: user.Id,
		Token:  token,
	}, nil
}

func newUser(req *pb.RegisterReq) (*model.UserProfile, error) {
	id, err := util.NextID()
	if err != nil {
		logx.Errorf("雪花算法生成id失败%v", err)
		return nil, errx.New(999, "雪花代码生成失败")
	}
	return &model.UserProfile{
		Id:       id,
		Username: req.Username,
		Password: req.Password,
		Phone: sql.NullString{
			String: req.Phone,
			Valid:  true,
		},
	}, nil
}
