-- 004_auth_hardening.sql 认证加固：JWT 版本号撤销 + Feed 查询索引
-- token_version 每次密码重置 +1；JWT claims 携带版本，中间件校验不一致即拒绝，
-- 使密码重置后所有旧会话立即失效。
-- 索引：Feed UNION 全表按时间扫（keyset 分页），为 question/answer 建独立时间索引。

ALTER TABLE `user`
  ADD COLUMN `token_version` INT NOT NULL DEFAULT 0 COMMENT 'JWT 版本，密码重置后 +1 使旧 token 失效';

ALTER TABLE `question`
  ADD INDEX `idx_question_time_only` (`create_time`);

ALTER TABLE `answer`
  ADD INDEX `idx_answer_time_only` (`create_time`);
