package logic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendVerifyCode_RedisClientMissing(t *testing.T) {
	svcCtx := &svc.ServiceContext{}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200001", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestSendVerifyCode_PhoneWindowIncrFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onIncr: func(key string) error {
			return errInjectedRedis
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200002", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_WindowExpireFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onExpire: func(key string) error {
			return errInjectedRedis
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200003", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_IPWindowIncrFailed(t *testing.T) {
	// 仅 IP 维度窗口失败：号码维度放行后命中注入点。
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onIncr: func(key string) error {
			if len(key) > len("verify:send:cnt:ip:") && key[:len("verify:send:cnt:ip:")] == "verify:send:cnt:ip:" {
				return errInjectedRedis
			}
			return nil
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200004", Type: 1, ClientIp: "10.1.1.1"})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_GlobalWindowIncrFailed(t *testing.T) {
	prefix := "verify:send:cnt:global:"
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onIncr: func(key string) error {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				return errInjectedRedis
			}
			return nil
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200005", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_GlobalQuotaExceeded(t *testing.T) {
	day := time.Now().Format("20060102")
	globalKey := fmt.Sprintf("verify:send:cnt:global:%s", day)
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{
			globalKey: fmt.Sprintf("%d", verifyCodeDailyGlobalLimit),
		}},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200006", Type: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.TooManyReq), "daily global quota must reject, got %v", err)
}

func TestSendVerifyCode_CooldownLookupFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onGet: func(key string) error {
			if key == "13800200007" {
				return errInjectedRedis
			}
			return nil
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200007", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_CooldownKeySetFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{"13800200008": "123456"}},
		onSetnxEx: func(key string) error {
			return errInjectedRedis
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200008", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestSendVerifyCode_StoreCodeFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onSetex: func(key string) error {
			if key == "13800200009" {
				return errInjectedRedis
			}
			return nil
		},
	}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	_, err := NewSendVerifyCodeLogic(context.Background(), svcCtx).
		SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800200009", Type: 1})
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}
