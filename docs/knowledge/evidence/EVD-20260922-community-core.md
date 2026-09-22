---
id: EVD-20260922-community-core
layer: evidence
title: 社区核心登录变更后的确定性重验
status: active
owner: agent
updated_at: 2026-09-22
scope:
- static
- unit
- integration
commands:
- make check
- make test
- make spec-evals-test
- make python-unit
- make integration-critical
- make generate
- git status --porcelain
- git diff --check
observed_commit: ea58747da1184847b9e7f470600b22054550e1b9
result: passed
coverage:
- requirements:
  - CORE-001
  - CORE-002
  - CORE-003
  - CORE-004
  - CORE-005
  - CORE-010
  - CORE-011
  - CORE-012
  - CORE-013
  - CORE-014
  - CORE-015
  - CORE-016
  - CORE-020
  - CORE-021
  - CORE-022
  - CORE-023
  - CORE-024
  - CORE-030
  - CORE-031
  - CORE-033
  - CORE-034
  - CORE-040
  - CORE-041
  - CORE-042
  - CORE-043
  - CORE-044
  - CORE-050
  - CORE-051
  - CORE-052
  - CORE-053
  - CORE-054
  - CORE-060
  - CORE-061
  - CORE-062
  - CORE-063
  - CORE-A01
  - CORE-A02
  - CORE-A03
  - CORE-A04
  - CORE-A05
  - CORE-A06
  - CORE-A07
  paths:
  - pkg/interceptor
  - app/content
  - app/interaction
  - app/message
  - app/media
  - app/user
  - app/gateway
  - pkg/idempotencyx
  - pkg/outboxx
  - pkg/visibilityx
  - proto/content/content.proto
  - proto/interaction/interaction.proto
  - proto/message/message.proto
  - proto/media/media.proto
  - proto/user/user.proto
  - go.mod
  - go.sum
---

# 社区核心登录变更后的确定性重验

命令均在 `ea58747da1184847b9e7f470600b22054550e1b9` 的源码、测试与配置上执行，返回 0。
该提交已包含登录锁定修复；`EVD-20260906-community-core` 因覆盖组含 `app/user` 而对当前树过期。
本次覆盖保留该组原有输入，并补入当前实现映射中的 `pkg/interceptor`、`proto/user/user.proto`
以及模块依赖 `go.mod` / `go.sum`。验证期间这些路径没有新的未提交改动。证据、实现矩阵和生成索引
在命令结束后单独提交。

知识检查使用主检出已有的 `.venv-knowledge`。生成使用既有工具：goctl 1.10.1、protoc 3.21.12、
`protoc-gen-go`、`protoc-gen-go-grpc`，以及固定的 grpcio-tools 1.71.0 / protobuf 5.29.4。
未安装新依赖，也未手改生成文件。`make generate` 后 `git status --porcelain` 为空，
`git diff --check` 通过。

| 实际验证 | 结果 |
| --- | --- |
| `make check` | 知识测试 40、工程 lint 测试 11、工程 lint、gofmt、`go vet`、golangci-lint 0 issues、govulncheck 符号漏洞 0。导入包另有 1 个、所需模块另有 2 个非可达告警 |
| `make test` | 113 个有测试包通过，启用 race 与包级覆盖率，0 失败 |
| 规格评测 / Python 工具 | 47 / 10+3 项通过 |
| `make integration-critical` | Interaction logic、User logic、User model 三个包通过。该目标未加 race |
| `make generate` | 工作树保持干净 |

`CORE-032` 不在本组。公开计数 30 秒收敛仍缺生产观测，实现矩阵保持 `unknown`。
本页不包含浏览器、设备、真实 provider、人类质量评审、完整 HTTP E2E、容量或生产月度观测。
原始日志在 `/tmp/community-core-verify.log`。
