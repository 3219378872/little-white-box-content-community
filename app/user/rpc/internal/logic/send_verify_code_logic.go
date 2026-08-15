package logic

import (
	"context"
	cr "crypto/rand"
	"fmt"
	"math/big"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendVerifyCodeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSendVerifyCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendVerifyCodeLogic {
	return &SendVerifyCodeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// SendVerifyCode 发送验证码
func (l *SendVerifyCodeLogic) SendVerifyCode(in *pb.SendVerifyCodeReq) (*pb.SendVerifyCodeResp, error) {
	if in.GetPhone() == "" || l.svcCtx == nil || l.svcCtx.RedisClient == nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// 冷却语义：仅当手机号仍持有未消费验证码时生效（防轰炸/高频重置）。
	// 验证码已被成功消费（注册/登录成功后删除）时允许立即重发——注册后
	// 马上验证码登录是正常流程，不应被 60 秒冷却阻断。
	existing, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.GetPhone())
	if err != nil {
		l.Errorw("Redis.GetCtx failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if existing != "" {
		cooldownKey := fmt.Sprintf("verify:cooldown:%s", in.GetPhone())
		first, setErr := l.svcCtx.RedisClient.SetnxExCtx(l.ctx, cooldownKey, "1", verifyCodeCooldownSeconds)
		if setErr != nil {
			l.Errorw("Redis.SetnxExCtx failed", logx.Field("err", setErr.Error()))
			return nil, errx.Wrap(setErr, errx.SystemError)
		}
		if !first {
			return nil, errx.NewWithCode(errx.TooManyReq)
		}
	}

	n, err := cr.Int(cr.Reader, big.NewInt(1000000))
	if err != nil {
		l.Errorw("crypto/rand.Int failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	randInt := n.Int64()

	// 十分钟过期
	expireTime := 60 * 10
	err = l.svcCtx.RedisClient.SetexCtx(l.ctx, in.GetPhone(), fmt.Sprintf("%06d", randInt), expireTime)

	if err != nil {
		l.Errorw("Redis.SetexCtx failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.SendVerifyCodeResp{}, nil
}

// verifyCodeCooldownSeconds 同一手机号验证码发送冷却（秒）。
const verifyCodeCooldownSeconds = 60
