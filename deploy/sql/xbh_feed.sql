CREATE DATABASE IF NOT EXISTS `xbh_feed` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `xbh_feed`;

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

CREATE TABLE IF NOT EXISTS `feed_outbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_author_post` (`author_id`, `post_id`),
  KEY `idx_author_created_post` (`author_id`, `created_at`, `post_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `feed_inbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_post` (`user_id`, `post_id`),
  KEY `idx_user_created_post` (`user_id`, `created_at`, `post_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
