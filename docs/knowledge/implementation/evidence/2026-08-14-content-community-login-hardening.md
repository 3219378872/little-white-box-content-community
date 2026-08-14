---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 100410d
commands:
  - go test -count=1 ./app/user/...
  - make check
  - make test
result: passed
---

# 2026-08-14 登录路径暴力尝试加固

## 缺陷

`LoginLogic` 两个分支均无暴力尝试限制（上一轮验证码加固只覆盖注册）：

1. 验证码登录（LoginType=2）可无限重试 6 位验证码——暴力破解后
   冒用登录。
2. 密码登录（LoginType=1）可无限试密码——在线爆破。

## 修复

- `register_logic.go`：提取共享 helper `recordVerifyCodeFailure` /
  `clearVerifyCodeFailures`（注册与登录共用 `verify:attempts:{phone}`
  计数，总错误次数 ≤5，达到上限作废验证码）。
- `login_logic.go`：
  - 验证码分支复用共享尝试限制；并补齐"验证码不存在/过期
    （`VerifyCodeExpired`）与错误（`VerifyCodeError`）的区分语义。
  - 密码分支新增 `loginFailureLocked`：`login:lock:{username}` 窗口
    10 分钟、上限 5 次，达到返回 `TooManyReq`；成功后清理计数。

## 测试

- `TestLoginVerifyCodeSharesAttemptLimitWithRegister`：登录错误 5 次
  后验证码作废（共享计数）。
- `TestLoginPasswordFailureLockout`：密码错误 5 次后返回 `TooManyReq`。
- 修复 staticcheck S1005（测试 fake 的 blank 赋值）。

## 结果

- `go test ./app/user/...` 全过；`make check` 通过（含 golangci-lint）；
  `make test` 全部模块通过（含 race）。

## 未覆盖边界

- 验证码/密码锁定基于 Redis 计数，Redis 故障时降级为不限制（纵深防御
  取舍，已记录）；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
