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
	n, err := cr.Int(cr.Reader, big.NewInt(1000000))
	if err != nil {
		l.Errorw("crypto/rand.Int failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	randInt := n.Int64()

	// 十分钟过期
	expireTime := 60 * 10
	err = l.svcCtx.RedisClient.SetexCtx(l.ctx, in.Phone, fmt.Sprintf("%06d", randInt), expireTime)

	if err != nil {
		l.Errorw("Redis.SetexCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.SendVerifyCodeResp{}, nil
}
