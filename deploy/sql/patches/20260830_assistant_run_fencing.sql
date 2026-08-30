-- Assistant worker lease fencing, frozen consent/input versions, journal ownership,
-- and database-enforced single terminal event. Safe to replay on an existing v3 volume.
USE xbh_assistant;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'lease_generation');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `lease_generation` BIGINT NOT NULL DEFAULT 0 AFTER `lease_owner`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'consent_version');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `consent_version` INT NOT NULL DEFAULT 0 AFTER `cancel_requested`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'input_version');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `input_version` BIGINT NOT NULL DEFAULT 1 AFTER `consent_version`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run_event' AND column_name = 'terminal_run_id');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run_event` ADD COLUMN `terminal_run_id` BIGINT DEFAULT NULL AFTER `type`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

DELETE e FROM `agent_run_event` e
JOIN (
  SELECT `run_id`, MAX(`id`) AS `keep_id`
  FROM `agent_run_event`
  WHERE `type` IN ('done', 'error')
  GROUP BY `run_id`
) terminal ON terminal.`run_id` = e.`run_id`
WHERE e.`type` IN ('done', 'error') AND e.`id` <> terminal.`keep_id`;

UPDATE `agent_run_event`
SET `terminal_run_id` = `run_id`
WHERE `type` IN ('done', 'error');

SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'agent_run_event' AND index_name = 'uk_run_terminal');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `agent_run_event` ADD UNIQUE INDEX `uk_run_terminal` (`terminal_run_id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_command_journal' AND column_name = 'run_id');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_command_journal` ADD COLUMN `run_id` BIGINT NOT NULL DEFAULT 0 AFTER `canonical_args_digest`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_command_journal' AND column_name = 'lease_generation');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_command_journal` ADD COLUMN `lease_generation` BIGINT NOT NULL DEFAULT 0 AFTER `run_id`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_command_journal' AND column_name = 'updated_at_ms');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_command_journal` ADD COLUMN `updated_at_ms` BIGINT NOT NULL DEFAULT 0 AFTER `created_at_ms`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

UPDATE `agent_command_journal`
SET `updated_at_ms` = `created_at_ms`
WHERE `updated_at_ms` = 0;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'memory_change' AND column_name = 'dedupe_request_id');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `memory_change` ADD COLUMN `dedupe_request_id` VARCHAR(64) GENERATED ALWAYS AS (CASE WHEN `request_id` IN ('''', ''anon'') THEN NULL ELSE `request_id` END) STORED AFTER `request_id`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

DELETE duplicate_change FROM `memory_change` duplicate_change
JOIN `memory_change` kept_change
  ON kept_change.`user_id` = duplicate_change.`user_id`
 AND kept_change.`dedupe_request_id` = duplicate_change.`dedupe_request_id`
 AND kept_change.`id` < duplicate_change.`id`
WHERE duplicate_change.`dedupe_request_id` IS NOT NULL;

SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'memory_change' AND index_name = 'uk_mem_change_command');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `memory_change` ADD UNIQUE INDEX `uk_mem_change_command` (`user_id`, `dedupe_request_id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
