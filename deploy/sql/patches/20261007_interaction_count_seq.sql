-- 互动计数按序号投影（CORE-032、REL-008）：Interaction 在 action_count 维护单调 count_seq，
-- 事件携带绝对计数与序号；Content 记录已投影的序号，只接受更大的序号，重复与乱序都收敛。
-- post.stats_seq 是计数快照的混合逻辑时钟，替代各进程本地时间，保证搜索计数补丁单调。
-- MySQL 8 无 ADD COLUMN IF NOT EXISTS，用 information_schema + 预处理语句保持幂等。

USE xbh_interaction;

-- action_count.count_seq：存量行从 0 起步，下一次计数变化即得到 1。
SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'action_count' AND column_name = 'count_seq');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `action_count` ADD COLUMN `count_seq` BIGINT NOT NULL DEFAULT 0 COMMENT ''计数序号：任一计数变化时在同一行锁内递增，随事件下发供下游按序覆盖'' AFTER `share_count`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

USE xbh_content;

-- post.interaction_seq：存量为 0，任何带序号的快照都会覆盖旧的增量计数。
SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'post' AND column_name = 'interaction_seq');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `post` ADD COLUMN `interaction_seq` BIGINT NOT NULL DEFAULT 0 COMMENT ''已投影的 Interaction 计数序号（action_count.count_seq），只接受更大的序号'' AFTER `share_count`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- post.stats_seq：存量为 0，首次计数变化取当前毫秒，不低于搜索中已有的时间戳序号。
SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'post' AND column_name = 'stats_seq');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `post` ADD COLUMN `stats_seq` BIGINT NOT NULL DEFAULT 0 COMMENT ''计数快照序号：计数变化时取 GREATEST(stats_seq+1, 当前毫秒)，供搜索按序覆盖'' AFTER `interaction_seq`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- comment.interaction_seq：评论只投影点赞数。
SET @col_exists := (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'comment' AND column_name = 'interaction_seq');
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `comment` ADD COLUMN `interaction_seq` BIGINT NOT NULL DEFAULT 0 COMMENT ''已投影的 Interaction 计数序号（action_count.count_seq），只接受更大的序号'' AFTER `reply_count`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
