-- Schema only. Run the documented paused-writer baseline before enabling consumers.
USE `xbh_interaction`;
SET @exists_count_revision := (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'xbh_interaction' AND table_name = 'action_count' AND column_name = 'revision');
SET @count_revision_sql := IF(@exists_count_revision = 0, 'ALTER TABLE action_count ADD COLUMN revision BIGINT NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE count_revision_stmt FROM @count_revision_sql;
EXECUTE count_revision_stmt;
DEALLOCATE PREPARE count_revision_stmt;
USE `xbh_content`;

-- Retain projection fences permanently; receipts for at least the supported
-- replay horizon (90 days). No automatic receipt deletion is installed.
CREATE TABLE IF NOT EXISTS `content_count_projection` (
 `target_type` VARCHAR(16) NOT NULL,
 `target_id` BIGINT NOT NULL,
 `revision` BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY (`target_type`, `target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE IF NOT EXISTS `content_count_receipt` (
 `event_id` BIGINT NOT NULL,
 `created_at` BIGINT NOT NULL,
 PRIMARY KEY (`event_id`), KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `content_count_projection_control` (
 `id` TINYINT NOT NULL, `legacy_reconciled` BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB;
INSERT IGNORE INTO `content_count_projection_control` (`id`, `legacy_reconciled`) VALUES (1, FALSE);
