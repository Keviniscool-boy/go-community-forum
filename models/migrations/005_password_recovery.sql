-- 005_password_recovery.sql 两段式密码重置凭据
-- 验证码只用于签发一次性重置凭据（reset_token），凭据摘要（sha256）与已用状态存 MySQL；
-- 消费凭据、改密、递增 token_version 在同一事务完成，凭据单次消费由 UPDATE ... WHERE used=0 保证。

CREATE TABLE IF NOT EXISTS `password_reset_token` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `email` varchar(255) COLLATE utf8mb4_general_ci NOT NULL,
  `token_hash` char(64) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'sha256(reset_token) hex 摘要',
  `expires_at` timestamp NOT NULL COMMENT '凭据过期时间（UTC）',
  `used` tinyint(4) NOT NULL DEFAULT 0,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_prt_email_used` (`email`, `used`, `expires_at`),
  KEY `idx_prt_token_hash` (`token_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
