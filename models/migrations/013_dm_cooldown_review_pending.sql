-- 013_dm_cooldown_review_pending.sql PRD 总规格 B02/B06 修复：
-- 会话拒绝时间（7 天冷却）+ 审阅 pending 唯一约束（替代 011 中含 create_time 的弱约束）

ALTER TABLE `conversation`
  ADD COLUMN `declined_at` timestamp NULL DEFAULT NULL COMMENT '最近一次拒绝时间（冷却计算基准）';

CREATE TABLE IF NOT EXISTS `article_review_pending` (
  `article_id` bigint(20) NOT NULL COMMENT '同一文章同时最多一个待处理审阅',
  `review_id` bigint(20) NOT NULL,
  `revision_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`article_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- 应用层事务：先 INSERT 此表（主键冲突即并发拒绝），成功后写入 review_request；决定后删除标记
