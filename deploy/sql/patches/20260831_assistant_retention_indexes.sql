-- REL-020: support bounded Assistant/Watch retention scans on existing volumes.
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

SET @has_watch_exec_created := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'watch_execution' AND index_name = 'idx_watch_exec_created'
);
SET @sql := IF(@has_watch_exec_created = 0,
  'ALTER TABLE `watch_execution` ADD INDEX `idx_watch_exec_created` (`created_at`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_watch_hit_created := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'watch_hit' AND index_name = 'idx_watch_hit_created'
);
SET @sql := IF(@has_watch_hit_created = 0,
  'ALTER TABLE `watch_hit` ADD INDEX `idx_watch_hit_created` (`created_at_ms`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
