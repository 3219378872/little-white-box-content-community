package logic

import (
	"context"
	"fmt"
	"testing"

	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 验证码发送频控：号码/IP/全局三维度窗口上限（纵深防御，接入真实 SMS 前生效）。
func TestSendVerifyCodeRateLimits(t *testing.T) {
	t.Run("手机号格式非法被拒绝", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
			SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "123", Type: 1})
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
	})

	t.Run("同一号码超过小时窗口上限被拒绝", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)
		for i := range verifyCodePhoneHourlyLimit {
			_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000010", Type: 1})
			require.NoError(t, err, "send %d should pass", i+1)
			// 模拟验证码被消费（注册/登录成功），绕开未消费冷却，
			// 单独压测号码维度小时窗口。
			_, err = mem.DelCtx(context.Background(), "13800000010")
			require.NoError(t, err)
		}
		_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000010", Type: 1})
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.TooManyReq), "phone hourly limit must reject, got %v", err)
	})

	t.Run("同一IP超过小时窗口上限被拒绝", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)
		for i := range verifyCodeIPHourlyLimit {
			// 每次换号码，隔离号码维度，只压 IP 维度。
			_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{
				Phone: fmt.Sprintf("13800001%03d", i), Type: 1, ClientIp: "10.0.0.1",
			})
			require.NoError(t, err, "send %d should pass", i+1)
		}
		_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{
			Phone: "13800009999", Type: 1, ClientIp: "10.0.0.1",
		})
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.TooManyReq), "ip hourly limit must reject, got %v", err)
	})

	t.Run("缺少client_ip时跳过IP维度不误伤", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)
		for i := range verifyCodeIPHourlyLimit + 1 {
			_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{
				Phone: fmt.Sprintf("13900002%03d", i), Type: 1,
			})
			require.NoError(t, err, "no client_ip must not trigger ip limit, send %d", i+1)
		}
	})
}
