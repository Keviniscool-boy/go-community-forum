-- 017_social_phase5.sql 第五阶段 M1（社交关系补全：评论编辑）（合流改号：原 main 线 009_social_phase5）
-- 说明：user_block 表由 frontend-v3 的 009_growth_personalization 创建（列名 blocker_id/blocked_id），
-- 本迁移只补评论编辑列。
-- 回滚说明：ALTER TABLE comment DROP COLUMN update_time。

-- 评论编辑（PRD F4）：update_time > create_time 即视为"已编辑"
ALTER TABLE `comment`
  ADD COLUMN `update_time` timestamp(3) NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '编辑时间（毫秒精度；>create_time 即已编辑）';
