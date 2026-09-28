---
id: EVD-20260928-cloudwego-query
layer: evidence
title: CloudWeGo 最终 HTTP 参数兼容验证
status: active
owner: agent
updated_at: '2026-09-28'
observed_commit: 4af041f7950991fc45636119213251d570c1a40d
result: passed
scope:
- static
- unit
commands:
- make check KNOWLEDGE_PYTHON=/home/dev/projects/little/little-white-box-content-community/.venv-knowledge/bin/python
- make test
- make coverage
artifacts:
- docs/knowledge/evidence/assets/cloudwego-query-20260928/validation-summary.json
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

# 最终 HTTP 参数兼容验证

固定观察提交上的 make check、完整 race 测试与覆盖率门禁均通过；真实 Hertz HTTP 决策表补充
三个搜索端点的空关键词错误码断言。参数绑定按原契约忽略空查询值：必填字符串为空时返回业务码
2，可选数值为空时保留零值，带默认值的参数为空时使用默认值；重复查询值选择第一个非空值。

这项回归由首次真实整栈 E2E 发现（147 passed、5 fixture skips、1 failed），当时空关键词错误
下沉到 Search RPC 并返回 5001。修复输入绑定并增加失败路径测试，没有修改 E2E 的接口断言。
真实整栈复验与浏览器记录由根仓后续证据承载。

完整隔离集成、etcd/存储故障与 Python 流水线记录在
[EVD-20260928-cloudwego-migration](EVD-20260928-cloudwego-migration.md) 的迁移主体提交；本次只改变
HTTP 查询绑定及对应测试，没有将旧集成执行冒充为当前提交的新执行。当前提交已完整复跑静态、race、
覆盖率与日志/错误隐私测试，CORE-054 与 REL-022 由本证据覆盖；生产、容量和设备门禁仍未验证。
