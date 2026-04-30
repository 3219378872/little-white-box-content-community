CREATE DATABASE IF NOT EXISTS xbh_analytics;

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
PARTITION BY cityHash64(event_id) % 64
ORDER BY event_id;

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.user_action_daily
ENGINE = AggregatingMergeTree()
ORDER BY (user_id, action, target_type, date)
AS SELECT
    toDate(event_time) AS date,
    user_id, action, target_type,
    uniqExactState(event_id) AS cnt
FROM xbh_analytics.behavior_events
GROUP BY date, user_id, action, target_type;

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.behavior_events_by_time
ENGINE = ReplacingMergeTree(event_time)
PARTITION BY cityHash64(event_id) % 64
ORDER BY event_id
AS SELECT * FROM xbh_analytics.behavior_events;
