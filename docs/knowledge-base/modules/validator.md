---
title: validator
tracks:
  - pkg/validator/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# validator

## 职责
输入校验：手机号、密码强度、用户名合法性，以及相关长度/格式上限常量。

## 公开接口与契约
- `ValidatePhone` / `IsPhoneValid` — 手机号校验。
- `ValidateUserName` / `IsUsernameValid` — 用户名校验。
- `CheckPasswordStrength` / `IsPasswordValid` — 密码强度校验。
- `limits.go` — 长度与格式上限常量。

## 上游
gateway 登录/注册 Logic、user 服务注册/资料更新 Logic。

## 下游
无（纯函数库）。

## 关键文件
- `validator.go` — 校验逻辑。
- `limits.go` — 边界常量。

## 注意事项与陷阱
- 校验失败应转为对应 `errx` 业务错误码返回给调用方，而非裸错误。
- 校验规则变更可能影响存量数据，调整前需评估兼容性。
