from __future__ import annotations

from datetime import datetime, timezone

from algorithm.offline_train.training import Sample


SAMPLE_QUERY = """
WITH
    toDateTime64(%(feature_start)s, 3) AS feature_start,
    toDateTime64(%(sample_start)s, 3) AS sample_start,
    toDateTime64(%(sample_end)s, 3) AS sample_end,
    item_stats AS (
        SELECT
            target_id,
            countIf(action = 'exposure') AS impressions,
            countIf(action = 'click') AS clicks,
            countIf(action IN ('like', 'favorite', 'comment', 'share')) AS positives,
            count() AS activity,
            min(event_time) AS first_seen
        FROM xbh_analytics.behavior_events FINAL
        WHERE event_time >= feature_start AND event_time < sample_start
          AND target_type = 'post'
        GROUP BY target_id
    ),
    positives AS (
        SELECT request_id, target_id, min(event_time) AS positive_time
        FROM xbh_analytics.behavior_events FINAL
        WHERE event_time >= sample_start AND event_time < sample_end
          AND action IN ('click', 'like', 'favorite', 'comment', 'share')
          AND target_type = 'post' AND request_id != ''
        GROUP BY request_id, target_id
    )
SELECT
    exposure.request_id AS request_id,
    toUnixTimestamp64Milli(exposure.event_time) AS event_time_ms,
    exposure.target_id AS post_id,
    if(positive.positive_time >= exposure.event_time
       AND positive.positive_time <= exposure.event_time + INTERVAL 1 DAY, 1, 0) AS label,
    1.0 / greatest(toFloat64(coalesce(exposure.position, 1)), 1.0) AS recall_score,
    if(stats.impressions > 0, stats.positives / stats.impressions, 0.0) AS quality,
    if(stats.impressions > 0, stats.clicks / stats.impressions, 0.0) AS ctr,
    if(stats.activity > 0,
       exp(-greatest(toFloat64(dateDiff('second', stats.first_seen, exposure.event_time)), 0.0)
           / 604800.0),
       1.0) AS freshness,
    log1p(toFloat64(stats.activity)) AS popularity,
    recall_score * 0.5 + quality * 0.18 + ctr * 0.12 AS coarse_score,
    exposure.recall_source AS category
FROM xbh_analytics.behavior_events AS exposure FINAL
LEFT JOIN item_stats AS stats ON stats.target_id = exposure.target_id
LEFT JOIN positives AS positive
    ON positive.request_id = exposure.request_id AND positive.target_id = exposure.target_id
WHERE exposure.event_time >= sample_start AND exposure.event_time < sample_end
  AND exposure.action = 'exposure' AND exposure.target_type = 'post'
  AND exposure.request_id != ''
ORDER BY exposure.event_time, exposure.request_id, exposure.position
"""


def load_samples(client, *, feature_start: datetime, sample_start: datetime, sample_end: datetime) -> list[Sample]:
    if not feature_start < sample_start < sample_end:
        raise ValueError("training windows must satisfy feature_start < sample_start < sample_end")
    result = client.query(
        SAMPLE_QUERY,
        parameters={
            "feature_start": _clickhouse_datetime(feature_start),
            "sample_start": _clickhouse_datetime(sample_start),
            "sample_end": _clickhouse_datetime(sample_end),
        },
    )
    columns = list(result.column_names)
    samples: list[Sample] = []
    for row in result.result_rows:
        values = dict(zip(columns, row, strict=True))
        samples.append(
            Sample(
                request_id=str(values["request_id"]),
                event_time_ms=int(values["event_time_ms"]),
                post_id=int(values["post_id"]),
                label=int(values["label"]),
                features={
                    name: float(values[name])
                    for name in (
                        "recall_score", "quality", "ctr", "freshness", "popularity", "coarse_score"
                    )
                },
                category=str(values["category"]),
            )
        )
    return samples


def _clickhouse_datetime(value: datetime) -> str:
    if value.tzinfo is None or value.utcoffset() is None:
        raise ValueError("training window timestamps must include an explicit timezone offset")
    return value.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
