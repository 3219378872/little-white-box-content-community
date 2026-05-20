CREATE DATABASE IF NOT EXISTS xbh_analytics;

-- 主表：以 user_id 为首列，优化用户维度聚合查询
-- spec: docs/superpowers/specs/2026-04-29-data-foundation-design.md §4.1
CREATE TABLE IF NOT EXISTS xbh_analytics.behavior_events (
    event_id    Int64,
    event_time  DateTime64(3),
    user_id     Int64,
    action      LowCardinality(String),
    target_id   Int64,
    target_type LowCardinality(String),
    duration    Int32 DEFAULT 0,
    scene       String DEFAULT '',
    client_ip   String DEFAULT ''
) ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (user_id, action, event_time, event_id);

-- 用户行为每日聚合：SummingMergeTree 在 merge 时自动累加 cnt
-- 注意：MV 在每次 INSERT 时触发，不做事件级去重；去重责任在 Bloom Filter（消费端）
--      与主表 ReplacingMergeTree（CK 侧）共同保障
CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.user_action_daily
ENGINE = SummingMergeTree()
ORDER BY (user_id, action, target_type, date)
AS SELECT
    toDate(event_time) AS date,
    user_id, action, target_type,
    count() AS cnt
FROM xbh_analytics.behavior_events
GROUP BY date, user_id, action, target_type;

-- 时间范围扫描优化视图：以 event_time 为首列，供 Spark 批量按时间窗口高效读取
CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.behavior_events_by_time
ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (event_time, user_id, event_id)
AS SELECT * FROM xbh_analytics.behavior_events;
