USE xbh_assistant;

CREATE TABLE IF NOT EXISTS `memory_target_lock` (
    `user_id` BIGINT NOT NULL,
    `target` VARCHAR(16) NOT NULL,
    PRIMARY KEY (`user_id`, `target`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Memory target 并发串行锁';

SET @watch_reserved_count_exists := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'watch_send_stat'
    AND column_name = 'reserved_count'
);
SET @watch_reserved_count_ddl := IF(
  @watch_reserved_count_exists = 0,
  'ALTER TABLE `watch_send_stat` ADD COLUMN `reserved_count` INT NOT NULL DEFAULT 0 AFTER `sent_count`',
  'SELECT 1'
);
PREPARE watch_reserved_count_stmt FROM @watch_reserved_count_ddl;
EXECUTE watch_reserved_count_stmt;
DEALLOCATE PREPARE watch_reserved_count_stmt;

CREATE TABLE IF NOT EXISTS `watch_send_reservation` (
    `bucket_id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `task_id` BIGINT NOT NULL,
    `period_kind` VARCHAR(16) NOT NULL,
    `period_start_ms` BIGINT NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`bucket_id`, `task_id`, `period_kind`),
    KEY `idx_watch_reservation_stat` (`user_id`, `task_id`, `period_kind`, `period_start_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 发送配额预留';
