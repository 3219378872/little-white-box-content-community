# SQL 资产约定

本目录有两类文件，交付路径不同：

## 基线 schema（`xbh_*.sql`）

除 `xbh_analytics.sql` 外，由 Compose 逐文件白名单挂载到 MySQL
`/docker-entrypoint-initdb.d`，**只在空数据卷首次初始化时按挂载目标名字典序执行**。
`xbh_analytics.sql` 只挂给 ClickHouse，禁止交给 MySQL。基线只允许
`CREATE TABLE IF NOT EXISTS` 级别的全量定义；不要把针对已有表的
`ALTER`/`UPDATE` 放在这里——存量卷不会重放它们，新卷上又可能先于依赖的表执行。

## 幂等补丁（`patches/*.sql`）

不进入 initdb.d 挂载（子目录不会被官方 entrypoint 递归执行），由本地编排
（根仓 stack.sh `apply_sql_patches`）在每次 `middleware-up` 时对现有数据卷逐个重放。
因此每个补丁必须自身幂等，推荐两种写法之一：

- 存在性守卫：用 `information_schema` 判断列/索引是否存在，再经预处理语句执行 DDL；
- 数据守卫：`UPDATE ... WHERE <旧缺陷特征>`，重复执行影响 0 行。

补丁由编排层经无默认库的 root 连接重放，因此每个补丁必须自带 `USE <schema>;`。

补丁合并进 patches/ 后即视为可对任意环境重复执行；不要再依赖「手工跑一次」。
ClickHouse 侧无此拆分：`xbh_analytics.sql` 全文幂等，直接整体重放。


## 行为去重升级（2026-09-30）

部署前先在隔离 ClickHouse/Redis 运行新增集成测试。`xbh_analytics.sql` 仍可整体幂等重放；新增
`behavior_facts` 普通视图并 `CREATE OR REPLACE` 下游查询视图，不删除或改写原始事件。先重放
schema，再升级 behaviorlog 聚合与 offline_train；新代码在视图缺失时失败，不退回未按曝光业务键
归一化的原始表。`behavior_events FINAL` 只解决 event_id 重投，不是曝光事实读取接口；统计、训练
及新增读取必须使用 `behavior_facts`，它按完整 `(request_id, target_id)` 选取最早 event_time、随后
最小 event_id 的一条曝光，其余动作继续按 event_id。丢失 INSERT ACK、并发和 Redis 收据丢失可能
留下多条原始 envelope，但不会成为多份事实。原 event_id/client_event_id 继续保留供追踪。

历史统计需要用已去重事实重新聚合：维护窗口内将 behaviorlog 的 `AggregateBackfillDays` 设为 90
回填有效保留窗，再恢复正常值。已有训练输出不会自动被替换，须基于新事实另行重训并走模型验收。
这些操作未由本代码修复自动执行，也不因视图升级就宣称历史统计/模型已修复。

推荐消费者的本次迁移仅支持 `Redis.Type=node`。`redisstore.ScanCtx` 调用的 go-redis 通用 SCAN 不会
遍历 Cluster 的全部 master；因此 `Type=cluster` 在配置校验阶段即拒绝启动，不构建客户端，不报告
迁移完成，也不启动消费。不要把一节点扫描成功当成全群集完成，也不要把 cluster 地址改标为 node
来绕过检查。Cluster 支持须另行实现并验证有界的逐 master 遍历及迁移，当前未提供。

推荐在线去重独立使用 `DedupTTL=7776000`，特征继续 `FeatureTTL=2592000`。停掉旧版推荐消费者后，
将 `LegacyDedupTTL` 配为该部署先前实际的 FeatureTTL（默认 2592000），再启动新版。新版在消费前
以有界 SCAN 与原子脚本升级仍存活的事件/曝光收据；到期点增加 `90天 - 旧TTL`，不按升级时间重新
计 90 天。`v3` 标记使重试/重启不续期；失败或超出 `DedupMigrationTimeoutSeconds`（默认 30 秒）
时启动失败，不继续消费。大键空间应预估并配置迁移预算，重启可安全继续已完成的部分。

已在旧 30 天策略下过期的收据不能凭空恢复；在开始历史重放前须依据可信原始事件恢复对应收据或
重建受影响特征。没有完成这一步时，历史 31–90 天重放保障仍是开放门禁，不能仅凭新 TTL 宣称补齐。
迁移过程中不能并行运行仍创建旧格式收据的消费者。新事件/仍可迁移收据的保留修复不等于找回丢失历史。
