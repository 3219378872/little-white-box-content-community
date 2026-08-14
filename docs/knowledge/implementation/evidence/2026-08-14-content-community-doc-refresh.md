---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: bea6c09
commands:
  - make check
  - make test
  - python3 scripts/engineering-lint.py
result: passed
---

# 2026-08-14 知识层元数据刷新（8 个修复批次后的复查）

## 背景

7af6c6a（分页归一化）至 bea6c09（登录加固）之间的 8 个修复批次
（inbox 分页、媒体孤儿对象、message 死方法、count-sync 重试、
幂等哈希、标签上限、验证码冷却、登录锁定）未同步刷新知识层
`verified_at/verified_commit` 元数据，导致台账/登记停留在旧提交。

## 本轮改动

- `IMP-content-community-backend`、`TRANSITION`、`IMP-todo-blocked-gates`：
  `verified_commit/observed_commit` 刷新到当前 HEAD。
- `IMP-architecture`、`IMP-engineering-conventions`、
  `IMP-development-quickstart`：`verified_commit` 刷新（内容与最新
  代码/Makefile 复查一致）。

## 台账内容复查（8 批次后，无需改行）

- CORE-032（partial）：count-sync 重试修复深化实现，但"30s 收敛未经
  生产观测证明"仍成立。
- CORE-050/051（aligned）：幂等哈希补齐 status/mediaIds 后描述更准确。
- CORE-004（aligned）：登录/注册加固不影响状态。
- REL-008（aligned）：90 天去重语义不变。
- 其余 aligned/partial 行与本轮修复无冲突。

## 结果

- `make check`、`make test`（84 包 0 失败）、engineering-lint 全过；
  26 份证据（含本轮）全部登记。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
