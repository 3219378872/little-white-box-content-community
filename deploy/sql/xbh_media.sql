-- 创建媒体数据库
CREATE DATABASE IF NOT EXISTS `xbh_media` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `xbh_media`;

-- 媒体文件表
CREATE TABLE IF NOT EXISTS `media` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '上传用户ID',
    `file_name` VARCHAR(255) NOT NULL COMMENT '文件名',
    `original_name` VARCHAR(255) DEFAULT NULL COMMENT '原始文件名',
    `file_type` VARCHAR(50) NOT NULL COMMENT '文件类型 image/video/audio',
    `mime_type` VARCHAR(100) DEFAULT NULL COMMENT 'MIME类型',
    `url` VARCHAR(500) NOT NULL COMMENT '访问URL',
    `thumbnail_url` VARCHAR(500) DEFAULT NULL COMMENT '缩略图URL',
    `storage_type` TINYINT DEFAULT 1 COMMENT '存储类型 1:MinIO 2:OSS 3:SeaweedFS',
    `bucket` VARCHAR(100) DEFAULT NULL COMMENT '存储桶',
    `object_key` VARCHAR(255) DEFAULT NULL COMMENT '对象键',
    `file_size` BIGINT DEFAULT 0 COMMENT '文件大小(字节)',
    `width` INT DEFAULT NULL COMMENT '宽度',
    `height` INT DEFAULT NULL COMMENT '高度',
    `duration` INT DEFAULT NULL COMMENT '时长(秒)',
    `format` VARCHAR(20) DEFAULT NULL COMMENT '格式',
    `bit_rate` INT DEFAULT NULL COMMENT '比特率',
    `status` TINYINT DEFAULT 1 COMMENT '状态 0:删除 1:正常 2:处理中',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_file_type` (`file_type`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='媒体文件表';

-- 媒体处理任务表
CREATE TABLE IF NOT EXISTS `media_task` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `media_id` BIGINT NOT NULL COMMENT '媒体ID',
    `task_type` TINYINT NOT NULL COMMENT '任务类型 1:压缩 2:转码 3:截图 4:水印',
    `status` TINYINT DEFAULT 0 COMMENT '状态 0:待处理 1:处理中 2:完成 3:失败',
    `progress` INT DEFAULT 0 COMMENT '进度',
    `error_msg` VARCHAR(500) DEFAULT NULL COMMENT '错误信息',
    `result` JSON DEFAULT NULL COMMENT '处理结果',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_media_id` (`media_id`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='媒体处理任务表';
