package logic

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"errx"
	"esx/pkg/validator"
	"fmt"
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
		l.Errorw("jwtx.GenerateToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
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
		l.Errorw("UserProfileModel.FindOneByPhone failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if phone != nil {
		return nil, errx.NewWithCode(errx.UserAlreadyExist)
	}

	code, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
	if err != nil {
		l.Errorw("Redis.GetCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if code == "" {
		return nil, errx.New(errx.VerifyCodeExpired, "验证码过期")
	}

	if code != in.VerifyCode {
		// 防暴力：验证码错误时递增尝试计数，超过上限立即作废验证码。
		attemptKey := fmt.Sprintf("verify:attempts:%s", in.Phone)
		attempts, incrErr := l.svcCtx.RedisClient.IncrCtx(l.ctx, attemptKey)
		if incrErr == nil {
			if attempts == 1 {
				_ = l.svcCtx.RedisClient.ExpireCtx(l.ctx, attemptKey, verifyCodeAttemptWindowSeconds)
			}
			if attempts >= verifyCodeMaxAttempts {
				_, _ = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
			}
		}
		return nil, errx.NewWithCode(errx.VerifyCodeError)
	}
	_, _ = l.svcCtx.RedisClient.DelCtx(l.ctx, fmt.Sprintf("verify:attempts:%s", in.Phone))

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
		l.Errorw("Redis.DelCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}

	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
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

	// 处理空用户名：直接用本函数生成的雪花 ID，天然全局唯一，
	// 避免 rand 撞名导致唯一索引冲突被误报为 UserAlreadyExist。
	if req.GetUsername() == "" {
		req.Username = fmt.Sprintf("小白盒用户%d", id)
	}

	// 处理密码，采用bcrypt算法
	var password string // 填充用户的密码
	if req.GetPassword() == "" {
		// 未提供密码时用 crypto/rand 生成 24 位十六进制随机密码，
		// 不再使用低熵 math/rand。
		randomBytes := make([]byte, 12)
		if _, randErr := rand.Read(randomBytes); randErr != nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		rawPass := "rp_" + hex.EncodeToString(randomBytes)
		password, err = util.HashPassword(rawPass)
		if err != nil {
			l.Errorw("util.HashPassword failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
	} else {
		password, err = util.HashPassword(req.Password)
		if err != nil {
			l.Errorw("util.HashPassword failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
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

// 验证码暴力尝试限制：窗口内允许的最大错误次数；达到后验证码作废。
const (
	verifyCodeMaxAttempts          = 5
	verifyCodeAttemptWindowSeconds = 600
)
