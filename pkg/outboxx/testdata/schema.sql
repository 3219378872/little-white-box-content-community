-- Mirrors the event_outbox table in deploy/sql/xbh_*.sql so claim and purge
-- queries run against the production index layout.
CREATE TABLE event_outbox (
    id BIGINT NOT NULL,
    topic VARCHAR(128) NOT NULL,
    tag VARCHAR(128) NOT NULL DEFAULT '',
    message_key VARCHAR(128) NOT NULL,
    payload LONGBLOB NOT NULL,
    status TINYINT NOT NULL DEFAULT 0,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at BIGINT NOT NULL DEFAULT 0,
    locked_by VARCHAR(128) NOT NULL DEFAULT '',
    locked_until BIGINT NOT NULL DEFAULT 0,
    last_error VARCHAR(1000) NOT NULL DEFAULT '',
    sent_at BIGINT DEFAULT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    KEY idx_event_outbox_ready (status, next_attempt_at, id),
    KEY idx_event_outbox_lease (status, locked_until, id),
    KEY idx_event_outbox_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
