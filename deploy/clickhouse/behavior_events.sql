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
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (user_id, action, event_time, event_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.user_action_daily
ENGINE = SummingMergeTree()
ORDER BY (user_id, action, target_type, date)
AS SELECT
    toDate(event_time) AS date,
    user_id, action, target_type,
    count() AS cnt
FROM xbh_analytics.behavior_events
GROUP BY date, user_id, action, target_type;

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.behavior_events_by_time
ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (event_time, user_id, event_id)
AS SELECT * FROM xbh_analytics.behavior_events;
