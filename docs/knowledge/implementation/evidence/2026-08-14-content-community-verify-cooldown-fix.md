---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 0fd53c7
commands:
  - make integration-critical
  - go test -count=1 -run TestRegisterByPhoneVerifyCodeCooldownAndAttemptLimit ./app/user/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 验证码冷却语义修复（集成测试回归）

## 缺陷

上一轮（登录加固）引入的 60 秒发送冷却按"任意 60 秒内一次"实现，
**拦截了合法场景**：注册成功消费验证码后，用户立即再发验证码登录
（注册后登录是正常流程）被 `TooManyReq` 阻断，`TestLoginPhoneIntegration`
（PR 核心集成门禁）因此失败。

## 修复

`send_verify_code_logic.go`：冷却仅在手机号仍持有**未消费验证码**时
生效——验证码存在时应用 60 秒冷却（防轰炸/高频重置）；验证码已被
成功消费（注册/登录成功删除）后允许立即重发。

## 测试

- 更新"发送冷却"单测为三分支：首次放行、未消费重发放行一次、未消费
  连续第三次被拦截；新增"验证码消费后允许立即重发"分支。
- `make integration-critical`：interaction/user logic/user model 全部
  通过（此前 user logic 失败）。

## 结果

- 集成门禁恢复；`make check`、`make test`（84 包 0 失败）全过。

## 未覆盖边界

- 冷却基于 Redis 计数，Redis 故障时降级为不限制（纵深防御取舍）；
  外部输入门禁不变，见 [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
