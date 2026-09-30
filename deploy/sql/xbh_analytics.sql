CREATE DATABASE IF NOT EXISTS xbh_analytics;

-- Raw accepted envelopes retain their original event IDs. FINAL converges
-- repeated delivery of one event; semantic consumers must read behavior_facts
-- below to collapse different event IDs describing one exposure.
CREATE TABLE IF NOT EXISTS xbh_analytics.behavior_events (
    event_id        Int64,
    client_event_id String,
    schema_version  UInt16,
    event_time      DateTime64(3),
    received_at     DateTime64(3),
    user_id         Int64 DEFAULT 0,
    anonymous_id    String DEFAULT '',
    session_id      String DEFAULT '',
    request_id      String DEFAULT '',
    action          LowCardinality(String),
    target_id       Int64,
    target_type     LowCardinality(String),
    scene           LowCardinality(String) DEFAULT '',
    position        Nullable(Int32),
    duration_ms     Nullable(Int64),
    recall_source   LowCardinality(String) DEFAULT '',
    model_version   LowCardinality(String) DEFAULT '',
    experiment_id   LowCardinality(String) DEFAULT '',
    producer        LowCardinality(String),
    client_ip       String DEFAULT '' COMMENT 'IP 的 SHA-256 哈希，不存完整 IP（REL-021）',
    client_version  LowCardinality(String) DEFAULT ''
) ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY event_id
TTL toDateTime(received_at) + INTERVAL 90 DAY DELETE;

-- REL-004/011: durable canonical facts, independent of Redis receipts. This
-- also repairs the read boundary for historical exposure duplicates. Earliest
-- event_time then event_id picks one deterministic attribution for each request/post.
CREATE OR REPLACE VIEW xbh_analytics.behavior_facts AS
SELECT *
FROM xbh_analytics.behavior_events FINAL
ORDER BY event_time, event_id
LIMIT 1 BY
    if(action = 'exposure' AND target_type = 'post', 'exposure', 'event'),
    if(action = 'exposure' AND target_type = 'post', request_id, ''),
    if(action = 'exposure' AND target_type = 'post', target_id, event_id);

-- Regular views aggregate the deduplicated raw facts at query time. An
-- insert-triggered materialized view would overcount at-least-once delivery.
CREATE OR REPLACE VIEW xbh_analytics.user_action_daily AS
SELECT
    toDate(event_time) AS date,
    user_id,
    action,
    target_type,
    count() AS cnt
FROM xbh_analytics.behavior_facts
GROUP BY date, user_id, action, target_type;

CREATE OR REPLACE VIEW xbh_analytics.behavior_events_by_time AS
SELECT *
FROM xbh_analytics.behavior_facts
ORDER BY event_time, user_id, event_id;

CREATE OR REPLACE VIEW xbh_analytics.behavior_events_by_scene AS
SELECT *
FROM xbh_analytics.behavior_facts
ORDER BY scene, event_time, event_id;

CREATE OR REPLACE VIEW xbh_analytics.behavior_events_by_model AS
SELECT *
FROM xbh_analytics.behavior_facts
ORDER BY model_version, experiment_id, event_time, event_id;

CREATE TABLE IF NOT EXISTS xbh_analytics.behavior_dead_letters (
    message_id  String,
    event_id    Int64 DEFAULT 0,
    payload     String,
    error       String,
    received_at DateTime64(3)
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(received_at)
ORDER BY (received_at, message_id)
TTL toDateTime(received_at) + INTERVAL 7 DAY DELETE;

-- REL-020：去标识聚合结果保留 365 天。ReplacingMergeTree(aggregated_at) 使定时
-- 聚合重复执行幂等；聚合读取 behavior_facts（已按事件/曝光业务键收敛），
-- 避免 at-least-once 投递在聚合侧重复计数。
CREATE TABLE IF NOT EXISTS xbh_analytics.daily_aggregates (
    date           Date,
    user_id        Int64,
    action         LowCardinality(String),
    target_type    LowCardinality(String),
    cnt            UInt64,
    aggregated_at  DateTime64(3)
) ENGINE = ReplacingMergeTree(aggregated_at)
PARTITION BY toYYYYMM(date)
ORDER BY (date, user_id, action, target_type)
TTL date + INTERVAL 365 DAY DELETE;
