---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 90a6a56
commands:
  - make model-pipeline-integration
  - make algorithm-test
  - make check
result: passed
---

# 2026-08-14 模型管线集成测试 TTL 冲突修复

## 缺陷

`make integration-all` / `make model-pipeline-integration` 稳定失败：
`algorithm/integration/test_model_pipeline.py` 第一次 `train_and_register`
成功、第二次失败（"training query returned no exposure samples"）。

根因：`deploy/sql/xbh_analytics.sql` 的 `behavior_events` 表
`TTL toDateTime(received_at) + INTERVAL 90 DAY DELETE`（REL-008 90 天）。
测试 seed 数据使用固定历史时间戳（2026-01），`received_at` 同样为
2026-01——插入时已"过期" 7 个月，ClickHouse 后台 TTL 删除任务在两次
查询之间异步删除了样本，导致第二次查询为空。生产 received_at 始终是
当前时间，无此问题（测试缺陷，非生产逻辑缺陷）。

## 修复

`test_model_pipeline.py`：训练窗口改为相对当前时间
（feature_start=now-14d、sample_start=now-7d、sample_end=now-1d），
seed 数据落在 90 天 TTL 内，两次训练查询均返回样本。

## 结果

- `make model-pipeline-integration`：15 项全过（此前稳定失败）；
  `make algorithm-test`、`make check` 通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
