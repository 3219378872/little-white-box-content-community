-- Add normalized provider usage anchors without rewriting historical totals.
USE `xbh_assistant`;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'cache_write_tokens');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `cache_write_tokens` BIGINT NOT NULL DEFAULT 0 AFTER `cache_tokens`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'reasoning_tokens');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `reasoning_tokens` BIGINT NOT NULL DEFAULT 0 AFTER `cache_write_tokens`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'last_prompt_tokens');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `last_prompt_tokens` BIGINT NOT NULL DEFAULT 0 AFTER `reasoning_tokens`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'agent_run' AND column_name = 'usage_estimated');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_run` ADD COLUMN `usage_estimated` TINYINT NOT NULL DEFAULT 0 AFTER `last_prompt_tokens`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
