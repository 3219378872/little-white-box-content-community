-- 创建Feed数据库
CREATE DATABASE IF NOT EXISTS `xbh_feed` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `xbh_feed`;

-- Feed流表
CREATE TABLE IF NOT EXISTS `feed` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '接收用户ID',
    `actor_id` BIGINT NOT NULL COMMENT '操作者ID',
    `type` TINYINT NOT NULL COMMENT '类型 1:发帖 2:评论 3:点赞 4:关注',
    `target_id` BIGINT NOT NULL COMMENT '目标ID',
    `target_type` TINYINT DEFAULT NULL COMMENT '目标类型',
    `content` VARCHAR(500) DEFAULT NULL COMMENT '内容摘要',
    `status` TINYINT DEFAULT 1 COMMENT '状态 0:删除 1:正常',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_actor_id` (`actor_id`),
    KEY `idx_type` (`type`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Feed流表';

-- 用户时间线表
CREATE TABLE IF NOT EXISTS `timeline` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `post_id` BIGINT NOT NULL COMMENT '帖子ID',
    `author_id` BIGINT NOT NULL COMMENT '作者ID',
    `score` DOUBLE DEFAULT 0 COMMENT '排序分数',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_user_post` (`user_id`, `post_id`),
    KEY `idx_user_score` (`user_id`, `score` DESC),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户时间线表';

-- 热门帖子缓存表
CREATE TABLE IF NOT EXISTS `hot_post` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `post_id` BIGINT NOT NULL COMMENT '帖子ID',
    `score` DOUBLE DEFAULT 0 COMMENT '热度分数',
    `decay_score` DOUBLE DEFAULT 0 COMMENT '衰减分数',
    `like_count` BIGINT DEFAULT 0 COMMENT '点赞数',
    `comment_count` BIGINT DEFAULT 0 COMMENT '评论数',
    `view_count` BIGINT DEFAULT 0 COMMENT '浏览数',
    `published_at` TIMESTAMP NULL COMMENT '发布时间',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_post_id` (`post_id`),
    KEY `idx_decay_score` (`decay_score` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='热门帖子缓存表';
