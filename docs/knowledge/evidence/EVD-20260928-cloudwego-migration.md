---
id: EVD-20260928-cloudwego-migration
layer: evidence
title: Kitex Hertz 全量迁移后端验证
status: active
owner: agent
updated_at: '2026-09-28'
observed_commit: ad42706db2e3a744da188b55f310e1314cf1be38
result: passed
scope:
- static
- unit
- integration
commands:
- make check KNOWLEDGE_PYTHON=/home/dev/projects/little/little-white-box-content-community/.venv-knowledge/bin/python
- make test
- make coverage
- INTEGRATION_ENV_NAME=xbh-cloudwego-test INTEGRATION_S3_PORT=28333 make integration-all
- PATH=/tmp/little-migrate/bin:/tmp/little-migrate/tooling/bin:$PATH make generate
- go list -m all
artifacts:
- docs/knowledge/evidence/assets/cloudwego-migration-20260928/validation-summary.json
- docs/knowledge/evidence/assets/cloudwego-migration-20260928/modules.txt
coverage:
- requirements:
  - CORE-054
  - REL-022
  paths:
  - app
  - pkg
  - kitex_gen
  - proto
  - deploy
  - algorithm
  - integration
  - scripts
  - Makefile
  - go.mod
  - go.sum
---

# Kitex / Hertz 后端迁移验证

固定观察提交上运行完整静态、race、覆盖率与隔离集成门禁，全部通过。完整集成覆盖真实 MySQL、
Redis、etcd、RocketMQ、ClickHouse、对象存储，以及 Python 模型训练、热加载和回滚流水线（15 项）。
生成检查在独立干净检出连续执行两次，均无漂移；模块图已无 go-zero。

新运行时的真实 socket 回归覆盖内部签名拒绝、grpc-go 互通、业务错误详情、响应脱敏与日志隐私、
服务端超时、流式半关闭/取消/长生命周期、etcd 实例替换与独立逻辑服务名。Hertz 回归覆盖 63 条
路由的决策表、精确 ID、参数 presence、可配置 HTTP 限制、分块上传、大于 20 MiB 视频、SSE 首帧
flush/游标恢复/断线。共享缓存的真实存储回归覆盖索引失效、事务回滚、负缓存以及提交后缓存故障。

覆盖率门禁：Handler 89.0%、Logic 78.0%、Model 15.2%、MQ consumer 72.4%、shared 60.4%。
漏洞扫描发现 0 个本代码可达漏洞；仍报告 2 个被导入包与 1 个依赖模块漏洞，但未发现本代码调用。
不把这一结果解释为整个依赖图不存在漏洞。当前 Go 版本下部分 CloudWeGo JSON 优化回退到标准实现，
本证据不包含性能或容量结论。

行为流水线的旧隔离 fixture 缺少必需签名和用户偏好 RPC，已补充带签名的独立测试服务；生产配置
校验未放宽。初轮 HTTP 配置回归的测试输入缺少必填 loginType，修正为契约有效输入后验证超时/零值。
最终结果来自固定提交上的完整重跑，不以修复前失败或并发编辑期间的结果替代。

本页只将错误隐私和业务日志条款恢复为 aligned；其他产品、设备、真人评审、容量与生产 SLO
仍按各 IMP 的独立验收边界保留 unknown/diverged。真实整栈、浏览器与外部模型观测记录在根仓 EVD。
