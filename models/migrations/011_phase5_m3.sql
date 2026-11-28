-- 011_phase5_m3.sql 第五阶段 M3（PRD F7 通知偏好 / F8 问题重定向）
-- 回滚说明：ALTER TABLE user DROP COLUMN notify_settings；ALTER TABLE question DROP COLUMN redirect_to。

-- 通知偏好（PRD F7）：按类型开关，JSON 列存于 user；NULL = 全部开启。
-- 键位：like(赞同/评论赞同) comment(评论) reply(回复评论) collect(收藏) dm(私信)；follow 不在可关列表。
ALTER TABLE `user`
  ADD COLUMN `notify_settings` json NULL COMMENT '通知偏好 JSON；NULL=全部开启';

-- 问题重定向（PRD F8 治理工具）：重复问题 A 重定向到 B；A 的回答保留可读；动作可逆（清空列）。
ALTER TABLE `question`
  ADD COLUMN `redirect_to` bigint NULL COMMENT '重定向目标问题 id（治理合并）；NULL=未重定向';
