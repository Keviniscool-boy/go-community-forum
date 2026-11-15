-- 001_auth_email.sql 邮箱认证迁移
-- 执行前注意：如果 user 表已有 email 为 NULL 的旧数据，先执行数据清洗再运行本迁移：
--   UPDATE user SET email = CONCAT('legacy_', user_id, '@placeholder.local') WHERE email IS NULL;
-- 回滚说明：本迁移只加列不改旧列语义，回滚时删除新增列与唯一索引即可：
--   ALTER TABLE user DROP INDEX uk_user_email,
--     DROP COLUMN email_verified, DROP COLUMN avatar_url, DROP COLUMN bio,
--     DROP COLUMN status, DROP COLUMN last_login_at;

ALTER TABLE `user`
  MODIFY `email` VARCHAR(255) COLLATE utf8mb4_general_ci NOT NULL,
  ADD COLUMN `email_verified` TINYINT NOT NULL DEFAULT 0 COMMENT '邮箱是否已验证',
  ADD COLUMN `avatar_url` VARCHAR(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '头像地址',
  ADD COLUMN `bio` VARCHAR(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '个人简介',
  ADD COLUMN `status` TINYINT NOT NULL DEFAULT 1 COMMENT '账号状态 1 正常 0 禁用',
  ADD COLUMN `last_login_at` TIMESTAMP NULL DEFAULT NULL COMMENT '最后登录时间',
  ADD UNIQUE KEY `uk_user_email` (`email`);
