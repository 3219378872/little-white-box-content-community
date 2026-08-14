---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 1de1bf7
commands:
  - make integration-all
  - make coverage
  - make coverage-target
result: passed
---

# 2026-08-14 全量套件复验（覆盖近四批改动）

## 背景

近四批改动（errx HTTP 状态映射 + GRPCCode/httpx 契约测试、媒体/评论幂等
哈希、上传哈希 ctx 透传、生成卫生）后，重跑全量集成套件作为最强验证。

## 结果

- `make integration-all`：**91 包 ok、0 失败，EXIT=0**（含全部
  integration-tagged 包与算法管线 offline_train/online_infer/model_registry
  15 项；环境自动清理）。
- `make coverage`：exit 0（基线门禁：handler 89.0≥88、logic 79.0≥76、
  model 12.7≥10、mq_consumer 74.4≥72、shared 49.1≥42）。
- `make coverage-target`：预期失败（目标门禁为愿景：90/85/70/80/80，未达；
  基线门禁为执行标准，见 code-quality 证据 2026-08-13）。

## 过程记录

- 首次 integration-all 在算法管线 Docker 构建处失败：
  `python:3.12-slim` 镜像源 `docker.m.daocloud.io` 连接被拒（瞬时外部镜像
  故障，非代码回归；全部 Go 集成测试当时已通过）。镜像恢复后
  `docker pull` 成功，重跑 EXIT=0。

## 未覆盖边界

- 外部输入门禁不变（真实月度 SLO、ASST 提升方向、DISC-062 复评），见
  IMP-todo-blocked-gates.md。
