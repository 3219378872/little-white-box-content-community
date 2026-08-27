-- DES-assistant-agent-runtime：存量卷补建 assistant 权威库与表（空卷由基线 xbh_assistant.sql 创建）。
-- 幂等，可重复执行；自带 USE，不依赖客户端默认库。

CREATE DATABASE IF NOT EXISTS `xbh_assistant` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `xbh_assistant`;

CREATE TABLE IF NOT EXISTS `user_preference` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `dimension` VARCHAR(64) NOT NULL COMMENT '偏好维度',
    `value` VARCHAR(191) NOT NULL COMMENT '维度取值',
    `score` DOUBLE NOT NULL DEFAULT 0 COMMENT '极性/分值，负值表示不喜欢',
    `source` VARCHAR(32) NOT NULL COMMENT 'behavior/conversation/explicit',
    `confidence` DOUBLE NOT NULL DEFAULT 0,
    `suppressed` TINYINT NOT NULL DEFAULT 0 COMMENT '1:不要记住这个',
    `history_json` JSON DEFAULT NULL COMMENT '被替换状态',
    `updated_at_ms` BIGINT NOT NULL COMMENT 'Unix 毫秒',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_pref_user_dim_value` (`user_id`, `dimension`, `value`),
    KEY `idx_pref_user_updated` (`user_id`, `updated_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Profile 记忆';

CREATE TABLE IF NOT EXISTS `user_interest` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `dimension` VARCHAR(64) NOT NULL,
    `value` VARCHAR(191) NOT NULL,
    `score` DOUBLE NOT NULL DEFAULT 0,
    `source` VARCHAR(32) NOT NULL,
    `confidence` DOUBLE NOT NULL DEFAULT 0,
    `suppressed` TINYINT NOT NULL DEFAULT 0,
    `last_event_at_ms` BIGINT NOT NULL COMMENT '衰减计时起点 Unix 毫秒',
    `history_json` JSON DEFAULT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_interest_user_dim_value` (`user_id`, `dimension`, `value`),
    KEY `idx_interest_user_event` (`user_id`, `last_event_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Interest 记忆';

CREATE TABLE IF NOT EXISTS `user_memory` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `happened_at_ms` BIGINT NOT NULL,
    `kind` VARCHAR(32) NOT NULL COMMENT 'ask/recommend/watch/other',
    `summary` VARCHAR(512) NOT NULL,
    `payload_json` JSON DEFAULT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_episodic_user_time` (`user_id`, `happened_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Episodic 记忆';

CREATE TABLE IF NOT EXISTS `memory_evidence` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `layer` VARCHAR(32) NOT NULL COMMENT 'profile/interest/episodic/task',
    `record_id` BIGINT NOT NULL,
    `source_kind` VARCHAR(32) NOT NULL,
    `source_ref` VARCHAR(191) DEFAULT NULL,
    `excerpt` VARCHAR(512) DEFAULT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_mem_ev_record` (`layer`, `record_id`),
    KEY `idx_mem_ev_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='记忆证据';

CREATE TABLE IF NOT EXISTS `task_memory` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `status` VARCHAR(16) NOT NULL DEFAULT 'open' COMMENT 'open/closed',
    `intent_text` VARCHAR(512) NOT NULL,
    `constraints_json` JSON DEFAULT NULL,
    `excluded_json` JSON DEFAULT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_task_user_status` (`user_id`, `status`, `updated_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Task 记忆';

CREATE TABLE IF NOT EXISTS `watch_task` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `condition_type` VARCHAR(32) NOT NULL COMMENT 'author_new_post/tag_new_post/keyword_new_post/post_revised/discussion_spike',
    `target_type` VARCHAR(16) NOT NULL COMMENT 'author/tag/keyword/post',
    `target_id` BIGINT NOT NULL DEFAULT 0,
    `target_text` VARCHAR(191) NOT NULL DEFAULT '',
    `enabled` TINYINT NOT NULL DEFAULT 1,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_watch_user_cond_target` (`user_id`, `condition_type`, `target_type`, `target_id`, `target_text`),
    KEY `idx_watch_enabled_type` (`enabled`, `condition_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 任务';

CREATE TABLE IF NOT EXISTS `watch_execution` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `task_id` BIGINT NOT NULL,
    `event_key` VARCHAR(191) NOT NULL,
    `hit` TINYINT NOT NULL DEFAULT 0,
    `used_llm` TINYINT NOT NULL DEFAULT 0,
    `status` VARCHAR(16) NOT NULL COMMENT 'matched/skipped/failed',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_watch_exec_event` (`task_id`, `event_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 匹配执行';

CREATE TABLE IF NOT EXISTS `watch_hit` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `task_id` BIGINT NOT NULL,
    `post_id` BIGINT NOT NULL DEFAULT 0,
    `title` VARCHAR(255) DEFAULT NULL,
    `summary` VARCHAR(512) DEFAULT NULL,
    `read_at_ms` BIGINT DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_watch_hit_user_read` (`user_id`, `read_at_ms`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 命中收件箱';

CREATE TABLE IF NOT EXISTS `agent_run` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `conversation_id` VARCHAR(64) DEFAULT NULL,
    `intent` VARCHAR(32) DEFAULT NULL,
    `model` VARCHAR(128) DEFAULT NULL,
    `latency_ms` INT NOT NULL DEFAULT 0,
    `status` VARCHAR(32) NOT NULL,
    `tool_summary_json` JSON DEFAULT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_agent_run_user_req` (`user_id`, `request_id`),
    KEY `idx_agent_run_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 运行审计';

CREATE TABLE IF NOT EXISTS `tool_call` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `run_id` BIGINT NOT NULL,
    `tool` VARCHAR(64) NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `arg_digest` VARCHAR(128) DEFAULT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_tool_call_run` (`run_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 工具调用审计';

CREATE TABLE IF NOT EXISTS `recommendation_feedback` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL DEFAULT '',
    `post_id` BIGINT NOT NULL,
    `reason` VARCHAR(32) NOT NULL COMMENT 'read/favorited/not_interested/too_long/other',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_rec_fb_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 推荐反馈';
