CREATE TABLE event_outbox (
    id BIGINT NOT NULL,
    topic VARCHAR(128) NOT NULL,
    tag VARCHAR(64) NOT NULL DEFAULT '',
    message_key VARCHAR(128) NOT NULL,
    payload BLOB NOT NULL,
    status TINYINT NOT NULL DEFAULT 0,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at BIGINT NOT NULL,
    locked_by VARCHAR(128) NOT NULL DEFAULT '',
    locked_until BIGINT NOT NULL DEFAULT 0,
    last_error VARCHAR(1000) NOT NULL DEFAULT '',
    sent_at BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    KEY idx_event_outbox_relay (status, next_attempt_at, locked_until, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
