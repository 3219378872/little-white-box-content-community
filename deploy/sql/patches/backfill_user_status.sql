-- 回填修复：register_logic newUser() 曾以 Go 零值 0 显式写入 user_profile.status，
-- 覆盖表默认值 1（正常），导致 SearchPublic（过滤 status=1）永远搜不到这些用户。
-- 全量置 1 的前提：status=0 的存量均为该缺陷产物，不存在主动封禁账号。
-- 幂等，可重复执行；自带 USE，不依赖客户端默认库。
USE xbh_user;

UPDATE `user_profile`
SET `status` = 1
WHERE `status` = 0;
