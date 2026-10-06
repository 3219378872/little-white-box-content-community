-- REL-020: support bounded Assistant retention scans on existing volumes.
USE `xbh_assistant`;

SET @has_message_retention := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'assistant_message' AND index_name = 'idx_msg_retention'
);
SET @sql := IF(@has_message_retention = 0,
  'ALTER TABLE `assistant_message` ADD INDEX `idx_msg_retention` (`created_at_ms`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
