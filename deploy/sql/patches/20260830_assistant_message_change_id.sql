-- memory-review system rows expose the authoritative undo change id.
USE `xbh_assistant`;

SET @change_id_exists := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = 'xbh_assistant'
    AND table_name = 'assistant_message'
    AND column_name = 'change_id'
);
SET @ddl := IF(
  @change_id_exists = 0,
  'ALTER TABLE `assistant_message` ADD COLUMN `change_id` BIGINT NOT NULL DEFAULT 0 AFTER `compacted`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
