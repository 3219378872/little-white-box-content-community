-- 迁移脚本：为 user_profile 新增 favorites_visibility 列
USE `xbh_user`;

ALTER TABLE `user_profile`
  ADD COLUMN `favorites_visibility` TINYINT NOT NULL DEFAULT 1 COMMENT '收藏列表可见性 1:公开 2:仅自己' AFTER `status`;
