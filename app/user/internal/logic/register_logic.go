package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	"esx/pkg/validator"
	"fmt"
	"jwtx"
	"math/rand"
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
	if in.Phone != "" {
		result, err := l.registerByPhone(in)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return l.registerByUserName(in)
}

func (l *RegisterLogic) registerByUserName(req *pb.RegisterReq) (*pb.RegisterResp, error) {
	_, err := validator.CheckPasswordStrength(req.Password)
	if err != nil {
		return nil, err
	}

	user, err := l.newUser(req)

	if err != nil {
		return nil, err
	}
	_, err = l.svcCtx.UserProfileModel.Insert(l.ctx, user)
	if err != nil {
		return nil, errx.NewWithCode(errx.UserAlreadyExist)
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

func (l *RegisterLogic) registerByPhone(in *pb.RegisterReq) (*pb.RegisterResp, error) {
	phone, err := l.svcCtx.UserProfileModel.FindOneByPhone(l.ctx, sql.NullString{
		String: in.Phone,
		Valid:  true,
	})
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}
	if phone != nil {
		return nil, errx.NewWithCode(errx.UserAlreadyExist)
	}

	code, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
	if err != nil {
		return nil, err
	}
	if code == "" {
		return nil, errx.New(errx.VerifyCodeExpired, "验证码过期")
	}

	if code != in.VerifyCode {
		return nil, errx.NewWithCode(errx.VerifyCodeError)
	}

	user, err := l.newUser(in)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.UserProfileModel.Insert(l.ctx, user)
	if err != nil {
		return nil, errx.NewWithCode(errx.UserAlreadyExist)
	}
	_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
	if err != nil {
		return nil, err
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

// 用于填充初始化内容
func (l *RegisterLogic) newUser(req *pb.RegisterReq) (*model.UserProfile, error) {
	id, err := util.NextID()
	if err != nil {
		l.Errorw("util.NextID snowflake id generation failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 处理空用户名
	if req.GetUsername() == "" {
		// TODO:此处采用rand简化流程，可采用redis分布式id设计
		req.Username = fmt.Sprintf("小白盒用户%d", rand.Intn(1000000))
	}

	// 处理密码，采用bcrypt算法
	var password string // 填充用户的密码
	if req.GetPassword() == "" {
		// 未提供密码时生成随机密码，不再使用硬编码默认值
		rawPass := fmt.Sprintf("rp_%d", rand.Intn(100000000))
		password, err = util.HashPassword(rawPass)
		if err != nil {
			return nil, err
		}
	} else {
		password, err = util.HashPassword(req.Password)
		if err != nil {
			return nil, err
		}
	}

	// 填充返回值
	return &model.UserProfile{
		Id:       id,
		Username: req.Username,
		Password: password,
		Phone: sql.NullString{
			String: req.Phone,
			Valid:  req.Phone != "",
		},
	}, nil
}
