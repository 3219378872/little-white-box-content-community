-- 创建消息数据库
CREATE DATABASE IF NOT EXISTS `xbh_message` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `xbh_message`;

-- 会话表
CREATE TABLE IF NOT EXISTS `conversation` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `target_user_id` BIGINT NOT NULL COMMENT '目标用户ID',
    `last_message` VARCHAR(500) DEFAULT NULL COMMENT '最后一条消息',
    `last_message_time` TIMESTAMP NULL COMMENT '最后消息时间',
    `unread_count` INT DEFAULT 0 COMMENT '未读数',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_users` (`user_id`, `target_user_id`),
    KEY `idx_target_user` (`target_user_id`),
    KEY `idx_last_message_time` (`last_message_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话表';

-- 私信表
CREATE TABLE IF NOT EXISTS `message` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `conversation_id` BIGINT NOT NULL COMMENT '会话ID',
    `sender_id` BIGINT NOT NULL COMMENT '发送者ID',
    `receiver_id` BIGINT NOT NULL COMMENT '接收者ID',
    `content` VARCHAR(1000) NOT NULL COMMENT '消息内容',
    `msg_type` TINYINT DEFAULT 1 COMMENT '消息类型 1:文本 2:图片 3:视频 4:语音',
    `status` TINYINT DEFAULT 0 COMMENT '状态 0:未读 1:已读',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    KEY `idx_conversation_id` (`conversation_id`),
    KEY `idx_sender_receiver_id` (`sender_id`, `receiver_id`, `id`),
    KEY `idx_receiver_sender_id` (`receiver_id`, `sender_id`, `id`),
    KEY `idx_receiver_status_sender` (`receiver_id`, `status`, `sender_id`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='私信表';

-- 系统通知表
CREATE TABLE IF NOT EXISTS `notification` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `type` TINYINT NOT NULL COMMENT '通知类型 1:点赞 2:评论 3:关注 4:系统 5:at',
    `title` VARCHAR(100) DEFAULT NULL COMMENT '标题',
    `content` VARCHAR(500) DEFAULT NULL COMMENT '内容',
    `target_id` BIGINT DEFAULT NULL COMMENT '目标ID',
    `target_type` TINYINT DEFAULT NULL COMMENT '目标类型',
    `sender_id` BIGINT DEFAULT NULL COMMENT '发送者ID',
    `status` TINYINT DEFAULT 0 COMMENT '状态 0:未读 1:已读',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_user_type_id` (`user_id`, `type`, `id`),
    KEY `idx_user_status` (`user_id`, `status`),
    KEY `idx_type` (`type`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统通知表';

-- 验证码表
CREATE TABLE IF NOT EXISTS `verify_code` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `phone` VARCHAR(20) NOT NULL COMMENT '手机号',
    `code` VARCHAR(10) NOT NULL COMMENT '验证码',
    `type` TINYINT NOT NULL COMMENT '类型 1:注册 2:登录 3:重置密码',
    `status` TINYINT DEFAULT 0 COMMENT '状态 0:未使用 1:已使用',
    `expire_at` TIMESTAMP NOT NULL COMMENT '过期时间',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    KEY `idx_phone_type` (`phone`, `type`),
    KEY `idx_expire_at` (`expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='验证码表';
