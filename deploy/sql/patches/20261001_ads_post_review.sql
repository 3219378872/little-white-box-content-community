-- SPEC-sponsored-ads ADS-014/ADS-030/ADS-031：存量 xbh_ad 补举报表与回扫、举报批次字段。
-- 空卷由基线 xbh_ad.sql 创建。幂等；自带 USE。
USE xbh_ad;

CREATE TABLE IF NOT EXISTS `ad_report` (
    `id` BIGINT NOT NULL,
    `ad_id` BIGINT NOT NULL,
    `revision` BIGINT NOT NULL COMMENT '举报时投放的过审 revision',
    `reporter_key` VARCHAR(64) NOT NULL COMMENT 'u:<userId> 或 s:<会话哈希>，不使用 anonymousId',
    `reason` VARCHAR(32) NOT NULL,
    `batch` VARCHAR(32) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_ad_report_reporter` (`ad_id`, `reporter_key`),
    KEY `idx_ad_report_batch` (`ad_id`, `batch`, `created_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告举报';

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'ad' AND column_name = 'approved_at_ms');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `ad` ADD COLUMN `approved_at_ms` BIGINT NOT NULL DEFAULT 0 COMMENT ''当前过审 revision 的结论时间，回扫据此跳过已按新代次审核的广告'' AFTER `published_revision`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'ad' AND column_name = 'rescan_generation');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `ad` ADD COLUMN `rescan_generation` VARCHAR(64) NOT NULL DEFAULT '''' COMMENT ''已处理的回扫代次（政策版本+种子变化计数）'' AFTER `approved_at_ms`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'ad' AND column_name = 'report_batch');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `ad` ADD COLUMN `report_batch` VARCHAR(32) NOT NULL DEFAULT '''' COMMENT ''未决举报批次；举报任务的 purpose_key'' AFTER `rescan_generation`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 存量过审广告缺少结论时间：以最近更新时间近似，避免补丁后首轮回扫把已按当前政策审核的广告全部重审。
UPDATE `ad` SET `approved_at_ms` = `updated_at_ms` WHERE `approved_at_ms` = 0 AND `approved_revision` > 0;
