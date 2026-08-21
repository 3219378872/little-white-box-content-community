package logic

import (
	"context"
	cr "crypto/rand"
	"fmt"
	"math/big"
	"time"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"errx"
	"esx/pkg/validator"

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

// 验证码发送频控（纵深防御，接入真实 SMS 前即生效）：
//   - 同一手机号每小时最多 verifyCodePhoneHourlyLimit 次（绝对上限，
//     与下方未消费冷却互补：注册后立即验证码登录的正常流程不受影响）；
//   - 同一客户端 IP 每小时最多 verifyCodeIPHourlyLimit 次（防跨号码轰炸）；
//   - 全站每日最多 verifyCodeDailyGlobalLimit 次（保护短信预算）。
const (
	verifyCodeCooldownSeconds      = 60
	verifyCodePhoneHourlyLimit     = 5
	verifyCodeIPHourlyLimit        = 20
	verifyCodeWindowSeconds        = 3600
	verifyCodeDailyGlobalLimit     = 20000
	verifyCodeDailyWindowSeconds   = 172800
)

// incrWindow 递增计数窗口；首次写入时设置过期。Redis 故障时返回错误，
// 由调用方按 SystemError 拒绝发送（验证码存储本身依赖 Redis，语义一致）。
func (l *SendVerifyCodeLogic) incrWindow(key string, windowSeconds int) (int64, error) {
	count, err := l.svcCtx.RedisClient.IncrCtx(l.ctx, key)
	if err != nil {
		return 0, err
	}
	if count == 1 {
		if err := l.svcCtx.RedisClient.ExpireCtx(l.ctx, key, windowSeconds); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// SendVerifyCode 发送验证码
func (l *SendVerifyCodeLogic) SendVerifyCode(in *pb.SendVerifyCodeReq) (*pb.SendVerifyCodeResp, error) {
	if in.GetPhone() == "" || l.svcCtx == nil || l.svcCtx.RedisClient == nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// RPC 层独立校验手机号格式（网关已校验，此处为纵深防御）。
	if err := validator.ValidatePhone(in.GetPhone()); err != nil {
		return nil, err
	}

	// 号码维度频控。
	phoneCount, err := l.incrWindow(fmt.Sprintf("verify:send:cnt:phone:%s", in.GetPhone()), verifyCodeWindowSeconds)
	if err != nil {
		l.Errorw("verify phone rate limit incr failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if phoneCount > verifyCodePhoneHourlyLimit {
		return nil, errx.NewWithCode(errx.TooManyReq)
	}

	// IP 维度频控：client_ip 缺失时跳过（降级为号码+全局维度）。
	if ip := in.GetClientIp(); ip != "" {
		ipCount, err := l.incrWindow(fmt.Sprintf("verify:send:cnt:ip:%s", ip), verifyCodeWindowSeconds)
		if err != nil {
			l.Errorw("verify ip rate limit incr failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
		if ipCount > verifyCodeIPHourlyLimit {
			return nil, errx.NewWithCode(errx.TooManyReq)
		}
	}

	// 全局日配额。
	day := time.Now().Format("20060102")
	globalCount, err := l.incrWindow(fmt.Sprintf("verify:send:cnt:global:%s", day), verifyCodeDailyWindowSeconds)
	if err != nil {
		l.Errorw("verify global rate limit incr failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if globalCount > verifyCodeDailyGlobalLimit {
		l.Errorw("verify code daily global quota exceeded",
			logx.Field("day", day), logx.Field("count", globalCount))
		return nil, errx.NewWithCode(errx.TooManyReq)
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
