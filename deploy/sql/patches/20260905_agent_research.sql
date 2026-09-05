-- Incremental research presentation and durable user interaction; no history reset.
USE `xbh_assistant`;
SET @source_type := (SELECT data_type FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_source_ledger' AND column_name = 'authority_id');
SET @ddl := IF(@source_type = 'varchar',
  'ALTER TABLE agent_source_ledger MODIFY COLUMN authority_id TEXT NOT NULL', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'client_protocol_version');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE agent_run ADD COLUMN client_protocol_version INT NOT NULL DEFAULT 1', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'agent_run_event' AND index_name = 'idx_run_type_seq');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE agent_run_event ADD INDEX idx_run_type_seq (run_id,type,seq)', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS agent_question_request (
  id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  run_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  message_id BIGINT NOT NULL,
  payload_json JSON NOT NULL,
  answer_request_id VARCHAR(128) NOT NULL DEFAULT '',
  answer_digest VARCHAR(64) NOT NULL DEFAULT '',
  PRIMARY KEY (run_id, id),
  KEY idx_question_message (message_id),
  CONSTRAINT fk_question_message FOREIGN KEY (message_id) REFERENCES assistant_message(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS agent_source_evidence (
  run_id BIGINT NOT NULL,
  handle VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  evidence_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  payload_json JSON NOT NULL,
  created_at_ms BIGINT NOT NULL,
  PRIMARY KEY (run_id, evidence_id),
  KEY idx_evidence_source (run_id, handle),
  KEY idx_evidence_retention (created_at_ms)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS assistant_message_presentation (
  message_id BIGINT NOT NULL,
  payload_json JSON NOT NULL,
  PRIMARY KEY (message_id),
  CONSTRAINT fk_presentation_message FOREIGN KEY (message_id) REFERENCES assistant_message(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
