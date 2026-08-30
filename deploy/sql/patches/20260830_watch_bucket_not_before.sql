-- Deferred Watch buckets retry at an explicit time without rewriting the unique merge window.
USE `xbh_assistant`;

SET @not_before_exists := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = 'xbh_assistant'
    AND table_name = 'watch_delivery_bucket'
    AND column_name = 'not_before_ms'
);
SET @ddl := IF(
  @not_before_exists = 0,
  'ALTER TABLE `watch_delivery_bucket` ADD COLUMN `not_before_ms` BIGINT NOT NULL DEFAULT 0 AFTER `window_start_ms`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
