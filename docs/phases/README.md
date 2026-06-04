# Go 微服务实施阶段总览

> 本目录包含 Go 微服务项目的分阶段可执行方案。完整技术方案请参阅 [../go-microservices-plan.md](../go-microservices-plan.md)。

## 阶段依赖关系

```
Phase 1 (基座搭建)
    │
    ├──→ Phase 2 (互动功能)
    │        │
    │        └──→ Phase 3 (搜索系统)
    │                 │
    │                 └──→ Phase 4 (推荐系统)
    │                          │
    │                          └──→ Phase 5 (运维监控)
    │
    └──→ Flutter 客户端 (与 Phase 2-5 并行)
```

## 阶段概览

| 阶段 | 名称 | 周期 | 核心交付 | 文档 |
|------|------|------|---------|------|
| Phase 1 | 基座搭建 | 6 周 | Gateway + User/Content/Media RPC | [phase-1-foundation.md](phase-1-foundation.md) |
| Phase 2 | 互动功能 | 5 周 | Interaction/Feed/Message + DTM | [phase-2-interaction.md](phase-2-interaction.md) |
| Phase 3 | 搜索系统 | 6 周 | ES + Milvus + 多路召回 | [phase-3-search.md](phase-3-search.md) |
| Phase 4 | 推荐系统 | 5 周 | Pipeline 漏斗 + 冷启动 | [phase-4-recommend.md](phase-4-recommend.md) |
| Phase 5 | 运维监控 | 4 周 | 监控 + 部署 + 安全 | [phase-5-ops.md](phase-5-ops.md) |

**总计**：26 周（约 6.5 个月）

## 技术栈总览

### 核心框架
- **语言**：Go 1.22+
- **微服务框架**：go-zero 1.6+
- **RPC**：gRPC + protobuf
- **服务注册**：etcd 3.5+

### 数据存储
- **数据库**：MySQL 8.0（每服务独立 Schema）
- **缓存**：Redis 7.x
- **搜索引擎**：Elasticsearch 8.x
- **向量数据库**：Milvus 2.x
- **对象存储**：MinIO / 阿里云 OSS

### 消息与事务
- **消息队列**：RocketMQ 5.x
- **分布式事务**：DTM 1.17+

### 可观测性
- **链路追踪**：Jaeger + OpenTelemetry
- **日志**：zap + Loki + Grafana
- **监控**：Prometheus + Grafana

### 前端
- **移动端**：Flutter 3.x + Dart

## 快速开始

### 前置条件
- Go 1.22+ 已安装
- Docker Desktop 已安装
- goctl 已安装（`go install github.com/zeromicro/go-zero/tools/goctl@latest`）

### 启动中间件
```bash
cd deploy
docker-compose -f docker-compose.middleware.yml up -d
```

### 开始 Phase 1
按照 [phase-1-foundation.md](phase-1-foundation.md) 开始基座搭建。

## 验收标准总览

### 功能验收
- [ ] 10 个微服务通过 etcd 注册发现
- [ ] Gateway 正常路由所有 API
- [ ] 核心流程：注册 → 登录 → 发帖 → 评论 → 点赞 → 收藏

### 性能验收
- [ ] 搜索多路召回 P99 < 100ms
- [ ] 推荐漏斗输出个性化结果
- [ ] 10 个服务总内存 < 1GB

### 质量验收
- [ ] 单元测试覆盖率 > 80%
- [ ] Jaeger 链路追踪覆盖所有 gRPC 调用
- [ ] Flutter App 核心页面流畅

## 文档约定

每个阶段文档包含以下结构：

1. **概述**：阶段目标、周期、前置条件
2. **详细任务清单**：按周拆分的任务
3. **技术要点**：该阶段相关的技术细节
4. **依赖与风险**：外部依赖和缓解措施
5. **验收标准**：功能、性能、测试验收

## 相关文档

- [Go 微服务技术方案](../go-microservices-plan.md) - 完整技术方案
- [Java 微服务技术方案](../java-microservices-plan.md) - Java 版本对比参考
