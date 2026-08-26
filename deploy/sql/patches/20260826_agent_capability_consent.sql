-- SPEC-assistant-agent-mode：Assistant Agent 能力授权表（存量库补建；空卷由基线 xbh_user.sql 创建）。
-- 幂等，可重复执行；自带 USE，不依赖客户端默认库。
USE xbh_user;

CREATE TABLE IF NOT EXISTS `agent_capability_consent` (
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `granted` TINYINT NOT NULL DEFAULT 0 COMMENT '1:已授权 0:未授权或已撤销',
    `granted_at` BIGINT DEFAULT NULL COMMENT '最近一次授权 Unix 毫秒',
    `revoked_at` BIGINT DEFAULT NULL COMMENT '最近一次撤销 Unix 毫秒（未撤销为 NULL）',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 能力授权表';
