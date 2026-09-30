-- Schema only: existing Search watermarks require the explicit baseline migration.
USE `xbh_content`;
SET @exists_stats_seq := (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'xbh_content' AND table_name = 'post' AND column_name = 'stats_seq');
SET @stats_seq_sql := IF(@exists_stats_seq = 0, 'ALTER TABLE post ADD COLUMN stats_seq BIGINT NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stats_seq_stmt FROM @stats_seq_sql;
EXECUTE stats_seq_stmt;
DEALLOCATE PREPARE stats_seq_stmt;
