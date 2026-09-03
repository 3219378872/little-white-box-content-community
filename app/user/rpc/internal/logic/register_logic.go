package logic

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"esx/app/user/rpc/internal/model"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
	"esx/pkg/util"
	"esx/pkg/validator"
	"fmt"

	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"

	"github.com/go-sql-driver/mysql"
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
		return nil, mapUserInsertError(err)
	}

	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	refreshToken, err := jwtx.GenerateRefreshToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateRefreshToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if err := storeRefreshJTI(l.ctx, l.svcCtx, refreshToken, user.Id); err != nil {
		return nil, err
	}

	return &pb.RegisterResp{
		UserId:       user.Id,
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (l *RegisterLogic) registerByPhone(in *pb.RegisterReq) (*pb.RegisterResp, error) {
	// 手机注册允许空密码（注册后由 newUser 生成随机密码），
	// 但显式提供的密码必须与用户名注册一样满足强度要求。
	if in.GetPassword() != "" {
		if _, err := validator.CheckPasswordStrength(in.Password); err != nil {
			return nil, err
		}
	}

	phone, err := l.svcCtx.UserProfileModel.FindOneByPhone(l.ctx, sql.NullString{
		String: in.Phone,
		Valid:  true,
	})
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Errorw("UserProfileModel.FindOneByPhone failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if phone != nil {
		return nil, errx.NewWithCode(errx.UserAlreadyExist)
	}

	code, err := l.svcCtx.RedisClient.GetCtx(l.ctx, verifyCodeRedisKey(in.Phone))
	if err != nil {
		l.Errorw("Redis.GetCtx failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if code == "" {
		return nil, errx.New(errx.VerifyCodeExpired, "验证码过期")
	}

	if code != in.VerifyCode {
		recordVerifyCodeFailure(l.ctx, l.svcCtx.RedisClient, in.Phone)
		return nil, errx.NewWithCode(errx.VerifyCodeError)
	}
	clearVerifyCodeFailures(l.ctx, l.svcCtx.RedisClient, in.Phone)

	user, err := l.newUser(in)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.UserProfileModel.Insert(l.ctx, user)
	if err != nil {
		return nil, mapUserInsertError(err)
	}
	if _, err = l.svcCtx.RedisClient.DelCtx(l.ctx, verifyCodeRedisKey(in.Phone)); err != nil {
		l.Errorw("Redis.DelCtx failed", logx.Field("err", err.Error()))
	}

	token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	refreshToken, err := jwtx.GenerateRefreshToken(user.Id, user.Username, l.svcCtx.Config.JwtConfig)
	if err != nil {
		l.Errorw("jwtx.GenerateRefreshToken failed", logx.Field("userId", user.Id), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if err := storeRefreshJTI(l.ctx, l.svcCtx, refreshToken, user.Id); err != nil {
		return nil, err
	}
	return &pb.RegisterResp{
		UserId:       user.Id,
		Token:        token,
		RefreshToken: refreshToken,
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
		// INSERT 显式携带全部列，零值会覆盖表默认值；status 必须显式置 1
		// （正常），否则 SearchPublic 的 status=1 过滤会让新用户永久不可搜索。
		Status: 1,
	}, nil
}

// 验证码暴力尝试限制：窗口内允许的最大错误次数；达到后验证码作废。
const (
	verifyCodeMaxAttempts          = 5
	verifyCodeAttemptWindowSeconds = 600
)

// recordVerifyCodeFailure 记录一次验证码校验失败；达到上限后作废验证码
// （注册与登录共享同一计数，总错误尝试受限）。Redis 故障时仅记录失败，
// 不阻断校验（防暴力是纵深防御）。
func recordVerifyCodeFailure(ctx context.Context, redis svc.RedisStore, phone string) {
	if redis == nil || phone == "" {
		return
	}
	attemptKey := fmt.Sprintf("verify:attempts:%s", phone)
	attempts, err := redis.IncrCtx(ctx, attemptKey)
	if err != nil {
		logx.WithContext(ctx).Errorw("verify attempts incr failed",
			logx.Field("err", err.Error()))
		return
	}
	if attempts == 1 {
		_ = redis.ExpireCtx(ctx, attemptKey, verifyCodeAttemptWindowSeconds)
	}
	if attempts >= verifyCodeMaxAttempts {
		_, _ = redis.DelCtx(ctx, verifyCodeRedisKey(phone))
	}
}

func verifyCodeRedisKey(phone string) string {
	return "verify:code:" + phone
}

func mapUserInsertError(err error) error {
	if isDuplicateKeyError(err) {
		return errx.NewWithCode(errx.UserAlreadyExist)
	}
	return errx.Wrap(err, errx.SystemError)
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// clearVerifyCodeFailures 校验成功后清理尝试计数。
func clearVerifyCodeFailures(ctx context.Context, redis svc.RedisStore, phone string) {
	if redis == nil || phone == "" {
		return
	}
	_, _ = redis.DelCtx(ctx, fmt.Sprintf("verify:attempts:%s", phone))
}
