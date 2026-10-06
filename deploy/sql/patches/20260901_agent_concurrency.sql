USE xbh_assistant;

CREATE TABLE IF NOT EXISTS `memory_target_lock` (
    `user_id` BIGINT NOT NULL,
    `target` VARCHAR(16) NOT NULL,
    PRIMARY KEY (`user_id`, `target`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Memory target 并发串行锁';
