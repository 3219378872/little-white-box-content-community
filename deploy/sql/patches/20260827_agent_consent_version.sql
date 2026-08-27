-- SPEC-assistant-agent-mode AGNT-007：存量库为 agent_capability_consent 补 consent_version。
-- 空卷由基线 xbh_user.sql 创建。幂等；自带 USE。
USE xbh_user;

SET @col_exists := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'agent_capability_consent'
    AND column_name = 'consent_version'
);
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE `agent_capability_consent` ADD COLUMN `consent_version` INT NOT NULL DEFAULT 1 COMMENT ''已授予的披露版本（AGNT-007）'' AFTER `revoked_at`',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
