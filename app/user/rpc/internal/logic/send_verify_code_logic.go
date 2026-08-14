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
	// 发送冷却：同一手机号 60 秒内只能请求一次，防验证码轰炸/重置攻击。
	cooldownKey := fmt.Sprintf("verify:cooldown:%s", in.GetPhone())
	first, err := l.svcCtx.RedisClient.SetnxExCtx(l.ctx, cooldownKey, "1", verifyCodeCooldownSeconds)
	if err != nil {
		l.Errorw("Redis.SetnxExCtx failed", logx.Field("phone", in.GetPhone()), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if !first {
		return nil, errx.NewWithCode(errx.TooManyReq)
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
		l.Errorw("Redis.SetexCtx failed", logx.Field("phone", in.GetPhone()), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.SendVerifyCodeResp{}, nil
}

// verifyCodeCooldownSeconds 同一手机号验证码发送冷却（秒）。
const verifyCodeCooldownSeconds = 60
