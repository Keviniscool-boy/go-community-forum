-- 014：问答解决闭环（产品审计 PRD §4）
-- 业务解决状态与审核状态（moderation_status）、可见性（visibility）严格分离：
--   resolution_status: 0=open（可回答） 1=solved（提问者已采纳答案） 2=closed（提问者关闭/重开由本列维护）
-- accepted_answer_id 可为 NULL；"答案属于本问题且公开" 无法用普通外键表达，由业务层事务内校验。
ALTER TABLE `question`
  ADD COLUMN `resolution_status` TINYINT NOT NULL DEFAULT 0 COMMENT '0=open 1=solved 2=closed',
  ADD COLUMN `accepted_answer_id` BIGINT NULL COMMENT '采纳的回答 id（业务层校验归属与可见性）',
  ADD COLUMN `version` BIGINT NOT NULL DEFAULT 1 COMMENT '并发更新版本（CAS）';

-- 清除采纳（回答删除/下架/作者禁用）按 accepted_answer_id 反查
CREATE INDEX `idx_question_accepted_answer` ON `question` (`accepted_answer_id`);
-- 我的内容管理（GET /me/content）按状态筛选问题列表
CREATE INDEX `idx_question_resolution_time` ON `question` (`resolution_status`, `status`, `create_time`);
