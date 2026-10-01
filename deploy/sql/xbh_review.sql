-- 审核平台权威库（SPEC-review-platform / DES-review-platform）
-- 时间均为 Unix 毫秒；结论、阶段结果与审计只追加。
CREATE DATABASE IF NOT EXISTS `xbh_review` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `xbh_review`;

CREATE TABLE IF NOT EXISTS `review_snapshot` (
    `hash` CHAR(64) NOT NULL COMMENT '规范化 JSON 的 sha256 hex',
    `biz_type` VARCHAR(32) NOT NULL COMMENT '业务类型',
    `content` MEDIUMTEXT NOT NULL COMMENT '规范化快照 JSON，写入后只读',
    `created_at` BIGINT NOT NULL,
    PRIMARY KEY (`hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='冻结快照';

CREATE TABLE IF NOT EXISTS `review_task` (
    `id` BIGINT NOT NULL,
    `biz_type` VARCHAR(32) NOT NULL,
    `object_id` BIGINT NOT NULL,
    `object_revision` BIGINT NOT NULL,
    `purpose` VARCHAR(16) NOT NULL COMMENT 'initial/qa/appeal/report/rescan',
    `purpose_key` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '回扫为政策版本，其余为空',
    `snapshot_hash` CHAR(64) NOT NULL,
    `market` VARCHAR(8) NOT NULL,
    `language` VARCHAR(8) NOT NULL,
    `industry` VARCHAR(32) NOT NULL DEFAULT '',
    `submitter_id` BIGINT NOT NULL COMMENT '提交主体（广告主）',
    `submission_seq` INT NOT NULL DEFAULT 0 COMMENT '该主体第几次 initial 送审，用于首次送审保护期',
    `required_role` VARCHAR(32) NOT NULL COMMENT '领取所需角色',
    `status` VARCHAR(16) NOT NULL COMMENT 'machine_pending/machine_running/human_pending/claimed/decided/superseded',
    `priority` INT NOT NULL DEFAULT 0,
    `deadline_ms` BIGINT NOT NULL,
    `lease_holder` BIGINT NOT NULL DEFAULT 0,
    `lease_generation` BIGINT NOT NULL DEFAULT 0,
    `lease_until_ms` BIGINT NOT NULL DEFAULT 0,
    `attempts` INT NOT NULL DEFAULT 0 COMMENT '人工领取次数',
    `excluded_reviewer` BIGINT NOT NULL DEFAULT 0 COMMENT '质检与申诉排除的原决策人',
    `source_task_id` BIGINT NOT NULL DEFAULT 0 COMMENT '质检与申诉对应的原任务',
    `escalation_reason` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '进入人审的原因',
    `policy_version` VARCHAR(64) NOT NULL DEFAULT '',
    `decision_id` BIGINT NOT NULL DEFAULT 0,
    `submitted_at_ms` BIGINT NOT NULL,
    `decided_at_ms` BIGINT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_review_task_object` (`biz_type`, `object_id`, `object_revision`, `purpose`, `purpose_key`),
    KEY `idx_review_task_queue` (`status`, `market`, `priority`, `deadline_ms`, `id`),
    KEY `idx_review_task_object` (`biz_type`, `object_id`, `status`),
    KEY `idx_review_task_submitter` (`biz_type`, `submitter_id`, `purpose`),
    KEY `idx_review_task_machine` (`status`, `lease_until_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='审核任务与人审队列';

CREATE TABLE IF NOT EXISTS `review_stage_result` (
    `id` BIGINT NOT NULL,
    `task_id` BIGINT NOT NULL,
    `stage` VARCHAR(32) NOT NULL COMMENT 'fingerprint/rules/router/ranker/decision',
    `component_version` VARCHAR(128) NOT NULL,
    `shadow` TINYINT NOT NULL DEFAULT 0,
    `outcome` VARCHAR(32) NOT NULL COMMENT 'hit/miss/pass/reject/human/degraded',
    `reason` VARCHAR(64) NOT NULL DEFAULT '',
    `output` MEDIUMTEXT NOT NULL,
    `latency_ms` BIGINT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_review_stage` (`task_id`, `stage`, `component_version`, `shadow`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='机审阶段记录';

CREATE TABLE IF NOT EXISTS `review_decision` (
    `id` BIGINT NOT NULL,
    `task_id` BIGINT NOT NULL,
    `biz_type` VARCHAR(32) NOT NULL,
    `object_id` BIGINT NOT NULL,
    `object_revision` BIGINT NOT NULL,
    `purpose` VARCHAR(16) NOT NULL,
    `verdict` VARCHAR(16) NOT NULL,
    `policy_codes` VARCHAR(1024) NOT NULL DEFAULT '[]',
    `policy_version` VARCHAR(64) NOT NULL,
    `source` VARCHAR(16) NOT NULL COMMENT 'machine/human/qa',
    `decided_by` BIGINT NOT NULL DEFAULT 0,
    `lease_generation` BIGINT NOT NULL DEFAULT 0,
    `note` VARCHAR(500) NOT NULL DEFAULT '',
    `decided_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_review_decision_task` (`task_id`),
    KEY `idx_review_decision_object` (`biz_type`, `object_id`, `object_revision`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='审核结论（只追加）';

CREATE TABLE IF NOT EXISTS `verdict_cache` (
    `snapshot_hash` CHAR(64) NOT NULL,
    `market` VARCHAR(8) NOT NULL,
    `policy_version` VARCHAR(64) NOT NULL,
    `verdict` VARCHAR(16) NOT NULL,
    `policy_codes` VARCHAR(1024) NOT NULL DEFAULT '[]',
    `source` VARCHAR(16) NOT NULL,
    `decision_id` BIGINT NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`snapshot_hash`, `market`, `policy_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='指纹复用';

CREATE TABLE IF NOT EXISTS `approved_media` (
    `sha256` CHAR(64) NOT NULL,
    `decision_id` BIGINT NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`sha256`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='已在人工通过快照中出现的图片哈希';

CREATE TABLE IF NOT EXISTS `review_seed` (
    `id` BIGINT NOT NULL,
    `issue_code` VARCHAR(64) NOT NULL,
    `market` VARCHAR(8) NOT NULL,
    `language` VARCHAR(8) NOT NULL,
    `text` VARCHAR(2000) NOT NULL,
    `status` VARCHAR(16) NOT NULL COMMENT 'candidate/active/retired',
    `source_task_id` BIGINT NOT NULL DEFAULT 0,
    `nominated_by` BIGINT NOT NULL,
    `confirmed_by` BIGINT NOT NULL DEFAULT 0,
    `retired_by` BIGINT NOT NULL DEFAULT 0,
    `index_state` VARCHAR(16) NOT NULL DEFAULT '' COMMENT '向量集合同步状态：空/indexed/removed',
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_review_seed_status` (`status`, `market`, `id`),
    KEY `idx_review_seed_index` (`index_state`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='种子库权威记录';

CREATE TABLE IF NOT EXISTS `reviewer` (
    `user_id` BIGINT NOT NULL,
    `roles` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '逗号分隔：reviewer,qa,policy_admin,qualification_reviewer',
    `markets` VARCHAR(256) NOT NULL DEFAULT '',
    `languages` VARCHAR(256) NOT NULL DEFAULT '',
    `active` TINYINT NOT NULL DEFAULT 1,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='审核角色与授权（运维脚本写入）';

-- 应用账号对本表只授予 INSERT/SELECT（根仓编排按表授权）。
CREATE TABLE IF NOT EXISTS `audit_log` (
    `id` BIGINT NOT NULL,
    `actor_id` BIGINT NOT NULL,
    `action` VARCHAR(64) NOT NULL,
    `object_type` VARCHAR(32) NOT NULL,
    `object_id` BIGINT NOT NULL,
    `before_state` VARCHAR(2000) NOT NULL DEFAULT '',
    `after_state` VARCHAR(2000) NOT NULL DEFAULT '',
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_audit_object` (`object_type`, `object_id`, `id`),
    KEY `idx_audit_actor` (`actor_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='只追加审计';

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
    `scope` VARCHAR(64) NOT NULL COMMENT '命令作用域',
    `user_id` BIGINT NOT NULL COMMENT '调用者用户ID',
    `key` VARCHAR(128) NOT NULL COMMENT '客户端幂等键',
    `command_hash` CHAR(64) NOT NULL COMMENT '命令参数指纹 sha256 hex',
    `resource_id` BIGINT NOT NULL COMMENT '命令成功产生的资源ID',
    `created_at` BIGINT NOT NULL COMMENT '创建 Unix 毫秒',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_scope_user_key` (`scope`, `user_id`, `key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='命令幂等表';
