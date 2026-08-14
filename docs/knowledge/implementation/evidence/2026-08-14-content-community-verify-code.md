---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 55e43d9
commands:
  - go test -count=1 ./app/user/...
  - make check
  - make test
result: passed
---

# 2026-08-14 验证码发送冷却与暴力尝试限制（安全加固）

## 缺陷

`SendVerifyCode` 无发送冷却、`registerByPhone` 无验证码尝试限制：

1. 攻击者可对任意手机号无限调用 `SendVerifyCode` 重置验证码
   （验证码轰炸/重置攻击面）。
2. 验证码错误可无限重试，6 位验证码可被暴力破解后冒用注册
   （受害者手机号被抢占）。

## 修复

- `send_verify_code_logic.go`：发送前 `SETNX verify:cooldown:{phone}`
  （TTL 60s）；存在则返回 `TooManyReq`。空手机号/未配置 Redis 返回
  `ParamError`。
- `register_logic.go`：验证码错误时 `INCR verify:attempts:{phone}`
  （首次设置 10 分钟窗口）；达到 `verifyCodeMaxAttempts=5` 后删除
  验证码（作废）；验证成功后清理尝试计数。
- `service_context.go`：`RedisStore` 接口扩展 `SetnxExCtx`/`IncrCtx`/
  `ExpireCtx`（go-zero `*redis.Redis` 原生支持，无新依赖）。

## 测试

- `TestRegisterByPhoneVerifyCodeCooldownAndAttemptLimit`：冷却第二次
  请求返回 `TooManyReq`；错误 5 次后验证码作废（后续返回
  `VerifyCodeExpired`）。
- `memoryRedis` 测试 fake 补齐 `GetCtx`/`SetnxExCtx`/`IncrCtx`/
  `ExpireCtx`。

## 结果

- `go test ./app/user/...` 全过；`make check` 通过；`make test` 全部
  模块通过（含 race）。

## 未覆盖边界

- 登录路径（`login_logic.go`）同样可暴力尝试验证码——登录有失败计数
  与锁定吗？见 IMP-architecture 追踪；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
