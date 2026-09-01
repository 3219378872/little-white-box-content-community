-- Assistant 权威库 v3：虚拟线程、异步 run、自然语言记忆、Watch 内部审计
-- （DES-assistant-agent-runtime / SPEC-assistant-agent）。
CREATE DATABASE IF NOT EXISTS `xbh_assistant` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `xbh_assistant`;

CREATE TABLE IF NOT EXISTS `runtime_marker` (
    `name` VARCHAR(64) NOT NULL,
    `applied_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Assistant schema 迁移标记';

CREATE TABLE IF NOT EXISTS `assistant_thread` (
    `user_id` BIGINT NOT NULL,
    `session_id` BIGINT NOT NULL DEFAULT 0,
    `unread_count` INT NOT NULL DEFAULT 0,
    `last_message_id` BIGINT NOT NULL DEFAULT 0,
    `last_message_preview` VARCHAR(255) NOT NULL DEFAULT '',
    `last_message_at_ms` BIGINT NOT NULL DEFAULT 0,
    `active_run_id` BIGINT NOT NULL DEFAULT 0,
    `updated_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='每用户一条虚拟线程';

CREATE TABLE IF NOT EXISTS `assistant_session` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `prompt_epoch` INT NOT NULL DEFAULT 1,
    `prompt_snapshot` MEDIUMBLOB,
    `tool_snapshot` MEDIUMBLOB,
    `compact_summary` MEDIUMTEXT,
    `status` VARCHAR(16) NOT NULL DEFAULT 'open',
    `successful_user_turns` INT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    `closed_at_ms` BIGINT DEFAULT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_session_user` (`user_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Assistant 会话与 prompt 快照';

CREATE TABLE IF NOT EXISTS `assistant_message` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `session_id` BIGINT NOT NULL,
    `run_id` BIGINT NOT NULL DEFAULT 0,
    `role` VARCHAR(16) NOT NULL,
    `kind` VARCHAR(32) NOT NULL DEFAULT 'message',
    `content` MEDIUMTEXT NOT NULL,
    `api_content` MEDIUMBLOB,
    `visible` TINYINT NOT NULL DEFAULT 1,
    `unread` TINYINT NOT NULL DEFAULT 0,
    `compacted` TINYINT NOT NULL DEFAULT 0,
    `change_id` BIGINT NOT NULL DEFAULT 0,
    `deleted_at_ms` BIGINT DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_msg_user_session` (`user_id`, `session_id`, `id`),
    KEY `idx_msg_user_created` (`user_id`, `deleted_at_ms`, `created_at_ms`),
    KEY `idx_msg_retention` (`created_at_ms`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='可见正文与 provider api_content 分离';

CREATE TABLE IF NOT EXISTS `agent_run` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `session_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `source` VARCHAR(32) NOT NULL,
    `status` VARCHAR(16) NOT NULL,
    `phase` VARCHAR(32) NOT NULL DEFAULT 'queued',
    `priority` INT NOT NULL DEFAULT 100,
    `queued_payload` JSON DEFAULT NULL,
    `lease_owner` VARCHAR(64) DEFAULT NULL,
    `lease_generation` BIGINT NOT NULL DEFAULT 0,
    `lease_until_ms` BIGINT DEFAULT NULL,
    `heartbeat_at_ms` BIGINT DEFAULT NULL,
    `cancel_requested` TINYINT NOT NULL DEFAULT 0,
    `consent_version` INT NOT NULL DEFAULT 0,
    `input_version` BIGINT NOT NULL DEFAULT 1,
    `prompt_epoch` INT NOT NULL DEFAULT 1,
    `model` VARCHAR(128) DEFAULT NULL,
    `rounds` INT NOT NULL DEFAULT 0,
    `tool_calls` INT NOT NULL DEFAULT 0,
    `input_tokens` BIGINT NOT NULL DEFAULT 0,
    `output_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_write_tokens` BIGINT NOT NULL DEFAULT 0,
    `reasoning_tokens` BIGINT NOT NULL DEFAULT 0,
    `last_prompt_tokens` BIGINT NOT NULL DEFAULT 0,
    `usage_estimated` TINYINT NOT NULL DEFAULT 0,
    `cost_usd` DOUBLE NOT NULL DEFAULT 0,
    `started_at_ms` BIGINT DEFAULT NULL,
    `ended_at_ms` BIGINT DEFAULT NULL,
    `last_activity_at_ms` BIGINT DEFAULT NULL,
    `error_code` VARCHAR(64) DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_agent_run_user_req` (`user_id`, `request_id`),
    KEY `idx_run_claim` (`status`, `priority`, `created_at_ms`, `lease_until_ms`),
    KEY `idx_run_user_status` (`user_id`, `status`, `source`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='异步 Agent run 与租约';

CREATE TABLE IF NOT EXISTS `agent_run_event` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `run_id` BIGINT NOT NULL,
    `seq` BIGINT NOT NULL,
    `type` VARCHAR(32) NOT NULL,
    `terminal_run_id` BIGINT DEFAULT NULL,
    `payload_json` JSON DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_run_seq` (`run_id`, `seq`),
    UNIQUE KEY `uk_run_terminal` (`terminal_run_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='持久 SSE 事件';

CREATE TABLE IF NOT EXISTS `agent_tool_call` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `run_id` BIGINT NOT NULL,
    `call_id` VARCHAR(64) NOT NULL,
    `tool` VARCHAR(64) NOT NULL,
    `args_json` JSON DEFAULT NULL,
    `canonical_args_digest` VARCHAR(128) NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `result_json` JSON DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_tool_call` (`run_id`, `call_id`),
    KEY `idx_tool_call_run` (`run_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='工具调用记录';

CREATE TABLE IF NOT EXISTS `agent_command_journal` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `tool` VARCHAR(64) NOT NULL,
    `canonical_args_digest` VARCHAR(128) NOT NULL,
    `run_id` BIGINT NOT NULL,
    `lease_generation` BIGINT NOT NULL,
    `result_json` JSON DEFAULT NULL,
    `status` VARCHAR(32) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_journal` (`user_id`, `request_id`, `tool`, `canonical_args_digest`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='工具副作用幂等日记';

CREATE TABLE IF NOT EXISTS `agent_source_ledger` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `run_id` BIGINT NOT NULL,
    `handle` VARCHAR(64) NOT NULL,
    `kind` VARCHAR(32) NOT NULL,
    `authority_id` VARCHAR(64) NOT NULL,
    `revision` BIGINT NOT NULL DEFAULT 0,
    `payload_json` JSON DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_source_handle` (`run_id`, `handle`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='本 run 来源 ledger';

CREATE TABLE IF NOT EXISTS `agent_confirmation` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `session_id` BIGINT NOT NULL,
    `run_id` BIGINT NOT NULL,
    `call_id` VARCHAR(64) NOT NULL,
    `tool` VARCHAR(64) NOT NULL,
    `canonical_args_digest` VARCHAR(128) NOT NULL,
    `target_revision` BIGINT NOT NULL,
    `status` VARCHAR(16) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `resolved_at_ms` BIGINT DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_confirm_run_call` (`run_id`, `call_id`),
    KEY `idx_confirm_user` (`user_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='delete_post 确认 CAS';

CREATE TABLE IF NOT EXISTS `assistant_input_command` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `session_id` BIGINT NOT NULL,
    `message_id` BIGINT NOT NULL,
    `run_id` BIGINT NOT NULL,
    `disposition` VARCHAR(16) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_input_command_user_req` (`user_id`, `request_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Assistant 输入接收幂等结果';

CREATE TABLE IF NOT EXISTS `agent_input_queue` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `run_id` BIGINT NOT NULL,
    `message_id` BIGINT NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_queue_run` (`run_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='busy 阶段 FIFO 输入';

CREATE TABLE IF NOT EXISTS `agent_run_alert` (
    `run_id` BIGINT NOT NULL,
    `level` VARCHAR(16) NOT NULL,
    `dimension` VARCHAR(32) NOT NULL,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`run_id`, `level`, `dimension`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='每 run 每级每维只告警一次';

CREATE TABLE IF NOT EXISTS `core_memory_entry` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `target` VARCHAR(16) NOT NULL,
    `content` TEXT NOT NULL,
    `content_norm` VARCHAR(512) NOT NULL,
    `version` INT NOT NULL DEFAULT 1,
    `deleted_at_ms` BIGINT DEFAULT NULL,
    `created_at_ms` BIGINT NOT NULL,
    `updated_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_mem_user_target` (`user_id`, `target`, `deleted_at_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='MEMORY/USER 自然语言条目';

CREATE TABLE IF NOT EXISTS `memory_target_lock` (
    `user_id` BIGINT NOT NULL,
    `target` VARCHAR(16) NOT NULL,
    PRIMARY KEY (`user_id`, `target`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Memory target 并发串行锁';

CREATE TABLE IF NOT EXISTS `memory_change` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `entry_id` BIGINT NOT NULL,
    `op` VARCHAR(16) NOT NULL,
    `before_json` JSON DEFAULT NULL,
    `after_json` JSON DEFAULT NULL,
    `result_version` INT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL,
    `dedupe_request_id` VARCHAR(64) GENERATED ALWAYS AS
      (CASE WHEN `request_id` IN ('', 'anon') THEN NULL ELSE `request_id` END) STORED,
    `undone` TINYINT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_mem_change_req` (`user_id`, `request_id`, `entry_id`, `op`),
    UNIQUE KEY `uk_mem_change_command` (`user_id`, `dedupe_request_id`),
    KEY `idx_mem_change_user` (`user_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='记忆变更与撤销';

CREATE TABLE IF NOT EXISTS `assistant_index_outbox` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `message_id` BIGINT NOT NULL,
    `op` VARCHAR(16) NOT NULL,
    `payload_json` JSON DEFAULT NULL,
    `published` TINYINT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_outbox_pub` (`published`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Assistant 历史 ES 派生 outbox';

CREATE TABLE IF NOT EXISTS `watch_task` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `condition_type` VARCHAR(32) NOT NULL,
    `target_type` VARCHAR(16) NOT NULL,
    `target_id` BIGINT NOT NULL DEFAULT 0,
    `target_text` VARCHAR(191) NOT NULL DEFAULT '',
    `enabled` TINYINT NOT NULL DEFAULT 1,
    `version` INT NOT NULL DEFAULT 1,
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
    `status` VARCHAR(16) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_watch_exec_event` (`task_id`, `event_key`),
    KEY `idx_watch_exec_created` (`created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 匹配执行审计';

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
    KEY `idx_watch_hit_user` (`user_id`, `created_at_ms`),
    KEY `idx_watch_hit_created` (`created_at_ms`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 内部命中（90 天审计，非用户收件箱）';

CREATE TABLE IF NOT EXISTS `watch_delivery_bucket` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `window_start_ms` BIGINT NOT NULL,
    `not_before_ms` BIGINT NOT NULL DEFAULT 0,
    `status` VARCHAR(16) NOT NULL,
    `hit_ids` JSON DEFAULT NULL,
    `run_id` BIGINT NOT NULL DEFAULT 0,
    `created_at_ms` BIGINT NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_bucket_user_window` (`user_id`, `window_start_ms`),
    KEY `idx_bucket_status` (`status`, `window_start_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 两分钟投递窗口';

CREATE TABLE IF NOT EXISTS `watch_send_stat` (
    `user_id` BIGINT NOT NULL,
    `task_id` BIGINT NOT NULL DEFAULT 0,
    `period_kind` VARCHAR(16) NOT NULL,
    `period_start_ms` BIGINT NOT NULL,
    `sent_count` INT NOT NULL DEFAULT 0,
    `reserved_count` INT NOT NULL DEFAULT 0,
    PRIMARY KEY (`user_id`, `task_id`, `period_kind`, `period_start_ms`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Watch 小时/日发送计数';

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

CREATE TABLE IF NOT EXISTS `recommendation_feedback` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `request_id` VARCHAR(64) NOT NULL DEFAULT '',
    `post_id` BIGINT NOT NULL,
    `reason` VARCHAR(32) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_rec_fb_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 推荐反馈';

INSERT IGNORE INTO `runtime_marker` (`name`, `applied_at_ms`) VALUES ('assistant_runtime_v3', 0);
