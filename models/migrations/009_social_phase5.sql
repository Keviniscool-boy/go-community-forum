-- 009_social_phase5.sql 第五阶段 M1（社交关系补全：用户拉黑 + 评论编辑）
-- 回滚说明：DROP TABLE user_block；ALTER TABLE comment DROP COLUMN update_time。

-- 用户拉黑（PRD F2）：拉黑后互相不可见、不可关注、不可私信。
-- 拉黑瞬间由业务层事务自动解除双向关注；uk_user_block 保证幂等。
CREATE TABLE IF NOT EXISTS `user_block` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL COMMENT '发起拉黑的人',
  `blocked_id` bigint NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_block` (`user_id`, `blocked_id`),
  KEY `idx_user_block_blocked` (`blocked_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 评论编辑（PRD F4）：update_time > create_time 即视为"已编辑"
ALTER TABLE `comment`
  ADD COLUMN `update_time` timestamp(3) NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '编辑时间（毫秒精度；>create_time 即已编辑）';
