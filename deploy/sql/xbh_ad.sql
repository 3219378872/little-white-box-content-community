-- 付费广告权威库（SPEC-sponsored-ads / DES-sponsored-ads）
-- 时间均为 Unix 毫秒。快照写入后只读；审核结论只经 review-decided 事件按 revision CAS 应用。
CREATE DATABASE IF NOT EXISTS `xbh_ad` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `xbh_ad`;

CREATE TABLE IF NOT EXISTS `advertiser` (
    `id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `name` VARCHAR(128) NOT NULL,
    `markets` VARCHAR(64) NOT NULL COMMENT '逗号分隔的经营市场',
    `revision` BIGINT NOT NULL DEFAULT 1,
    `approved_revision` BIGINT NOT NULL DEFAULT 0,
    `review_status` VARCHAR(16) NOT NULL COMMENT 'pending_review/approved/rejected',
    `policy_codes` VARCHAR(1024) NOT NULL DEFAULT '[]',
    `approved_name` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '最近过审的主体名称',
    `review_task_id` BIGINT NOT NULL DEFAULT 0,
    `submitted_at_ms` BIGINT NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_advertiser_user` (`user_id`),
    KEY `idx_advertiser_reconcile` (`review_status`, `review_task_id`, `submitted_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告主';

CREATE TABLE IF NOT EXISTS `advertiser_qualification` (
    `id` BIGINT NOT NULL,
    `advertiser_id` BIGINT NOT NULL,
    `market` VARCHAR(8) NOT NULL,
    `industry` VARCHAR(32) NOT NULL,
    `document_media_id` BIGINT NOT NULL COMMENT '私有存储中的证件',
    `valid_until_ms` BIGINT NOT NULL,
    `status` VARCHAR(16) NOT NULL COMMENT 'pending/approved/rejected/expired',
    `submitted_revision` BIGINT NOT NULL COMMENT '随哪个广告主 revision 送审',
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_qualification_advertiser` (`advertiser_id`, `market`, `industry`),
    KEY `idx_qualification_expiry` (`status`, `valid_until_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='行业资质';

CREATE TABLE IF NOT EXISTS `ad` (
    `id` BIGINT NOT NULL,
    `advertiser_id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `revision` BIGINT NOT NULL DEFAULT 1,
    `approved_revision` BIGINT NOT NULL DEFAULT 0,
    `review_status` VARCHAR(16) NOT NULL COMMENT 'draft/pending_review/approved/rejected/appealing',
    `serving_status` VARCHAR(16) NOT NULL DEFAULT 'none' COMMENT 'none/serving/paused/offline',
    `market` VARCHAR(8) NOT NULL,
    `industry` VARCHAR(32) NOT NULL,
    `start_ms` BIGINT NOT NULL DEFAULT 0,
    `end_ms` BIGINT NOT NULL DEFAULT 0,
    `policy_codes` VARCHAR(1024) NOT NULL DEFAULT '[]',
    `pause_reason` VARCHAR(64) NOT NULL DEFAULT '',
    `appealed_revision` BIGINT NOT NULL DEFAULT 0,
    `review_task_id` BIGINT NOT NULL DEFAULT 0,
    `submitted_at_ms` BIGINT NOT NULL DEFAULT 0,
    `published_revision` BIGINT NOT NULL DEFAULT 0 COMMENT '素材已发布到公开路径的过审 revision',
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_ad_owner` (`user_id`, `id`),
    KEY `idx_ad_reconcile` (`review_status`, `review_task_id`, `submitted_at_ms`),
    KEY `idx_ad_serving` (`serving_status`, `market`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告';

CREATE TABLE IF NOT EXISTS `ad_snapshot` (
    `ad_id` BIGINT NOT NULL,
    `revision` BIGINT NOT NULL,
    `advertiser_name` VARCHAR(128) NOT NULL,
    `title` VARCHAR(100) NOT NULL,
    `body` VARCHAR(500) NOT NULL,
    `cta` VARCHAR(32) NOT NULL,
    `landing_url` VARCHAR(2048) NOT NULL,
    `landing_domain` VARCHAR(255) NOT NULL,
    `media` VARCHAR(2048) NOT NULL DEFAULT '[]' COMMENT 'JSON [{mediaId, sha256}]，mediaId 为 ad_asset.id',
    `market` VARCHAR(8) NOT NULL,
    `language` VARCHAR(8) NOT NULL,
    `industry` VARCHAR(32) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`ad_id`, `revision`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告 revision 快照（只读）';

-- 素材与证件只写入私有存储；过审后由 ad-mq 按 ads/<sha256> 复制到公开路径（ADS-015）。
CREATE TABLE IF NOT EXISTS `ad_asset` (
    `id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `kind` VARCHAR(16) NOT NULL COMMENT 'creative/document',
    `sha256` CHAR(64) NOT NULL,
    `mime_type` VARCHAR(64) NOT NULL,
    `size_bytes` BIGINT NOT NULL,
    `object_key` VARCHAR(255) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_ad_asset_owner` (`user_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='私有素材与证件';

CREATE TABLE IF NOT EXISTS `event_outbox` (
    `id` BIGINT NOT NULL COMMENT '事件ID',
    `topic` VARCHAR(128) NOT NULL COMMENT 'RocketMQ Topic',
    `tag` VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'RocketMQ Tag',
    `message_key` VARCHAR(128) NOT NULL COMMENT '端到端幂等键',
    `payload` LONGBLOB NOT NULL COMMENT '原始事件载荷',
    `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0待发送 1发送中 2已发送 3待重试 4死信',
    `attempts` INT NOT NULL DEFAULT 0 COMMENT '投递尝试次数',
    `next_attempt_at` BIGINT NOT NULL DEFAULT 0 COMMENT '下次重试 Unix 毫秒',
    `locked_by` VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'relay 租约持有者',
    `locked_until` BIGINT NOT NULL DEFAULT 0 COMMENT 'relay 租约到期 Unix 毫秒',
    `last_error` VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '最近一次投递错误',
    `sent_at` BIGINT DEFAULT NULL COMMENT '发送成功 Unix 毫秒',
    `created_at` BIGINT NOT NULL COMMENT '创建 Unix 毫秒',
    `updated_at` BIGINT NOT NULL COMMENT '更新 Unix 毫秒',
    PRIMARY KEY (`id`),
    KEY `idx_event_outbox_ready` (`status`, `next_attempt_at`, `id`),
    KEY `idx_event_outbox_lease` (`status`, `locked_until`, `id`),
    KEY `idx_event_outbox_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='事务事件发件箱';

CREATE TABLE IF NOT EXISTS `idempotency` (
    `id` BIGINT NOT NULL COMMENT '幂等记录ID',
    `scope` VARCHAR(64) NOT NULL COMMENT '命令作用域，如 ad:create/ad:update',
    `user_id` BIGINT NOT NULL COMMENT '调用者用户ID',
    `key` VARCHAR(128) NOT NULL COMMENT '客户端幂等键',
    `command_hash` CHAR(64) NOT NULL COMMENT '命令参数指纹 sha256 hex',
    `resource_id` BIGINT NOT NULL COMMENT '命令成功产生的资源ID',
    `created_at` BIGINT NOT NULL COMMENT '创建 Unix 毫秒',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_scope_user_key` (`scope`, `user_id`, `key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='命令幂等表';
