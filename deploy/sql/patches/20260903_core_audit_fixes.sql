-- Idempotent schema fixes from the 2026-09-03 non-Agent backend audit.
-- Safe to replay on volumes created from the previous baselines.

-- CORE-022: comment body is 1-2000 Unicode chars.
USE `xbh_content`;
ALTER TABLE `comment`
    MODIFY COLUMN `content` VARCHAR(2000) NOT NULL COMMENT '评论内容';

SET @post_media_ids_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = 'xbh_content'
      AND table_name = 'post'
      AND column_name = 'media_ids'
);
SET @post_media_ids_sql := IF(
    @post_media_ids_exists = 0,
    'ALTER TABLE `post` ADD COLUMN `media_ids` JSON DEFAULT NULL COMMENT ''引用的已上传媒体ID列表（CORE-024）'' AFTER `images`',
    'SELECT 1'
);
PREPARE post_media_ids_stmt FROM @post_media_ids_sql;
EXECUTE post_media_ids_stmt;
DEALLOCATE PREPARE post_media_ids_stmt;

-- CORE-041: conversation preview must accept the same 1000-char body as message.content.
USE `xbh_message`;
ALTER TABLE `conversation`
    MODIFY COLUMN `last_message` VARCHAR(1000) DEFAULT NULL COMMENT '最后一条消息';

-- Persist the independent thumbnail object key so cleanup can delete both S3 objects.
USE `xbh_media`;
SET @thumb_key_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = 'xbh_media'
      AND table_name = 'media'
      AND column_name = 'thumbnail_object_key'
);
SET @thumb_key_sql := IF(
    @thumb_key_exists = 0,
    'ALTER TABLE `media` ADD COLUMN `thumbnail_object_key` VARCHAR(255) DEFAULT NULL COMMENT ''缩略图对象键'' AFTER `object_key`',
    'SELECT 1'
);
PREPARE thumb_key_stmt FROM @thumb_key_sql;
EXECUTE thumb_key_stmt;
DEALLOCATE PREPARE thumb_key_stmt;
