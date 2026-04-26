-- 创建内容数据库
CREATE DATABASE IF NOT EXISTS `xbh_content` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `xbh_content`;

CREATE TABLE IF NOT EXISTS `dtm_barrier` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `trans_type` VARCHAR(45) NOT NULL DEFAULT '',
    `gid` VARCHAR(128) NOT NULL DEFAULT '',
    `branch_id` VARCHAR(128) NOT NULL DEFAULT '',
    `op` VARCHAR(45) NOT NULL DEFAULT '',
    `barrier_id` VARCHAR(45) NOT NULL DEFAULT '',
    `reason` VARCHAR(45) NOT NULL DEFAULT '',
    `create_time` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `update_time` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_barrier` (`gid`, `branch_id`, `op`, `barrier_id`),
    KEY `idx_create_time` (`create_time`),
    KEY `idx_update_time` (`update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='DTM branch barrier table';

-- 帖子表
CREATE TABLE IF NOT EXISTS `post` (
    `id` BIGINT NOT NULL COMMENT '帖子ID',
    `author_id` BIGINT NOT NULL COMMENT '作者ID',
    `title` VARCHAR(200) NOT NULL COMMENT '标题',
    `content` TEXT NOT NULL COMMENT '内容',
    `images` JSON DEFAULT NULL COMMENT '图片列表',
    `video_url` VARCHAR(255) DEFAULT NULL COMMENT '视频URL,未使用',
    `cover_url` VARCHAR(255) DEFAULT NULL COMMENT '封面URL,未使用',
    `status` TINYINT DEFAULT 1 COMMENT '状态 0:草稿 1:已发布 2:已删除 3:审核中',
    `view_count` BIGINT DEFAULT 0 COMMENT '浏览数',
    `like_count` BIGINT DEFAULT 0 COMMENT '点赞数',
    `comment_count` BIGINT DEFAULT 0 COMMENT '评论数',
    `favorite_count` BIGINT DEFAULT 0 COMMENT '收藏数',
    `share_count` BIGINT DEFAULT 0 COMMENT '分享数',
    `is_top` TINYINT DEFAULT 0 COMMENT '是否置顶',
    `is_hot` TINYINT DEFAULT 0 COMMENT '是否热门',
    `is_essence` TINYINT DEFAULT 0 COMMENT '是否精华',
    `category_id` BIGINT DEFAULT NULL COMMENT '分类ID',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `published_at` TIMESTAMP NULL COMMENT '发布时间',
    PRIMARY KEY (`id`),
    KEY `idx_author_id` (`author_id`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`),
    KEY `idx_published_at` (`published_at`),
    KEY `idx_like_count` (`like_count`),
    KEY `idx_view_count` (`view_count`),
    KEY `idx_category_id` (`category_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='帖子表';

-- 帖子标签关联表
CREATE TABLE IF NOT EXISTS `post_tag` (
    `id` BIGINT NOT NULL,
    `post_id` BIGINT NOT NULL COMMENT '帖子ID',
    `tag_name` VARCHAR(50) NOT NULL COMMENT '标签名',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_post_tag` (`post_id`, `tag_name`),
    KEY `idx_tag_name` (`tag_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='帖子标签关联表';

-- 标签表
CREATE TABLE IF NOT EXISTS `tag` (
    `id` BIGINT NOT NULL,
    `name` VARCHAR(50) NOT NULL COMMENT '标签名',
    `description` VARCHAR(200) DEFAULT NULL COMMENT '标签描述',
    `icon` VARCHAR(255) DEFAULT NULL COMMENT '标签图标',
    `post_count` BIGINT DEFAULT 0 COMMENT '帖子数',
    `follow_count` BIGINT DEFAULT 0 COMMENT '关注数',
    `status` TINYINT DEFAULT 1 COMMENT '状态 0:禁用 1:正常',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_name` (`name`),
    KEY `idx_post_count` (`post_count`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='标签表';

-- 评论表
CREATE TABLE IF NOT EXISTS `comment` (
    `id` BIGINT NOT NULL COMMENT '评论ID',
    `post_id` BIGINT NOT NULL COMMENT '帖子ID',
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `parent_id` BIGINT DEFAULT NULL COMMENT '父评论ID NULL:一级评论',
    `reply_user_id` BIGINT DEFAULT NULL COMMENT '回复用户ID',
    `content` VARCHAR(1000) NOT NULL COMMENT '评论内容',
    `images` JSON DEFAULT NULL COMMENT '图片列表',
    `like_count` BIGINT DEFAULT 0 COMMENT '点赞数',
    `reply_count` BIGINT DEFAULT 0 COMMENT '回复数',
    `status` TINYINT DEFAULT 1 COMMENT '状态 0:删除 1:正常 2:审核中',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_post_id` (`post_id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_parent_id` (`parent_id`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评论表';

-- 帖子分类表
CREATE TABLE IF NOT EXISTS `category` (
    `id` BIGINT NOT NULL,
    `name` VARCHAR(50) NOT NULL COMMENT '分类名',
    `description` VARCHAR(200) DEFAULT NULL COMMENT '分类描述',
    `icon` VARCHAR(255) DEFAULT NULL COMMENT '分类图标',
    `sort_order` INT DEFAULT 0 COMMENT '排序',
    `parent_id` BIGINT DEFAULT 0 COMMENT '父分类ID',
    `status` TINYINT DEFAULT 1 COMMENT '状态',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='帖子分类表';
