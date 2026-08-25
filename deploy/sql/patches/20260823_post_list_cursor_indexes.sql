-- 帖子列表 keyset 游标分页（CORE-061）所需复合索引。
-- MySQL 8 无 CREATE INDEX IF NOT EXISTS，用 information_schema + 预处理语句实现幂等，
-- 可对旧数据卷重复执行（与 backfill_user_status.sql 同一策略）。
USE xbh_content;

-- 全局列表·最新 (status, created_at, id)
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'post' AND index_name = 'idx_post_status_created_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `post` ADD INDEX `idx_post_status_created_id` (`status`, `created_at`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 全局列表·热门 (status, like_count, id)
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'post' AND index_name = 'idx_post_status_like_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `post` ADD INDEX `idx_post_status_like_id` (`status`, `like_count`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 全局列表·浏览量 (status, view_count, id)
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'post' AND index_name = 'idx_post_status_view_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `post` ADD INDEX `idx_post_status_view_id` (`status`, `view_count`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 用户主页·最新 (author_id, status, created_at, id)
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'post' AND index_name = 'idx_post_author_status_created_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `post` ADD INDEX `idx_post_author_status_created_id` (`author_id`, `status`, `created_at`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 用户主页·热门 (author_id, status, like_count, created_at, id)
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'post' AND index_name = 'idx_post_author_status_like_created_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `post` ADD INDEX `idx_post_author_status_like_created_id` (`author_id`, `status`, `like_count`, `created_at`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
