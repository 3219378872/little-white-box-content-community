-- SPEC-agent-watch WCH-021: existing Watch tables need optimistic-lock versions.
-- Safe to replay on volumes created before the v3 Assistant baseline.
USE `xbh_assistant`;

SET @watch_version_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = 'xbh_assistant'
      AND table_name = 'watch_task'
      AND column_name = 'version'
);
SET @watch_version_sql := IF(
    @watch_version_exists = 0,
    'ALTER TABLE `watch_task` ADD COLUMN `version` INT NOT NULL DEFAULT 1 AFTER `enabled`',
    'SELECT 1'
);
PREPARE watch_version_stmt FROM @watch_version_sql;
EXECUTE watch_version_stmt;
DEALLOCATE PREPARE watch_version_stmt;
