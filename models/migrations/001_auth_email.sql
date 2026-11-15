-- 001_auth_email.sql 邮箱认证迁移（对旧库：先清洗 NULL 邮箱，再加列与唯一索引）
-- 旧用户邮箱占位为 user_id 派生的 placeholder.invalid 地址（RFC 2606 保留 TLD），
-- 该地址不是用户验证过的真实邮箱，email_verified 保持 0，不允许以其作为登录邮箱。
-- 回滚说明：删除新增列与唯一索引即可（占位邮箱行按 email_verified=0 识别）。

-- 1. NULL / 空邮箱清洗（仅旧库需要，幂等；此时 email_verified 列还不存在，
--    后面 ALTER 加列时 DEFAULT 0 即保证占位邮箱用户不被视为已验证）
UPDATE `user`
   SET email = CONCAT('legacy_', user_id, '@placeholder.invalid')
 WHERE email IS NULL OR email = '';

-- 2. 加列（对已按新结构初始化的库，执行到这里之前版本表会跳过本迁移；
--    只有真正从旧基线升级的库才会执行下面的 ALTER）
ALTER TABLE `user`
  MODIFY `email` VARCHAR(255) COLLATE utf8mb4_general_ci NOT NULL,
  ADD COLUMN `email_verified` TINYINT NOT NULL DEFAULT 0 COMMENT '邮箱是否已验证',
  ADD COLUMN `avatar_url` VARCHAR(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '头像地址',
  ADD COLUMN `bio` VARCHAR(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '个人简介',
  ADD COLUMN `status` TINYINT NOT NULL DEFAULT 1 COMMENT '账号状态 1 正常 0 禁用',
  ADD COLUMN `last_login_at` TIMESTAMP NULL DEFAULT NULL COMMENT '最后登录时间',
  ADD UNIQUE KEY `uk_user_email` (`email`);
