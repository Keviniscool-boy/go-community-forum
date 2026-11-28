-- 015：回答邀请闭环（待回答问题发现与回答邀请 PRD §8）
-- 状态机：0=pending 1=accepted 2=declined 3=answered 4=revoked 5=expired
-- 唯一性：同一 (question_id, invitee_id) 同时最多一条"有效邀请"（pending/accepted）。
--   MySQL 无部分唯一索引，用生成列实现：有效行 active_key=1（同对只允许一条），
--   终态行 active_key=invitation_id（互不冲突，保留历史、支持过期后重新邀请）。
CREATE TABLE IF NOT EXISTS `question_answer_invitation` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `invitation_id` bigint NOT NULL COMMENT '雪花id',
  `question_id` bigint NOT NULL,
  `inviter_id` bigint NOT NULL COMMENT '问题作者',
  `invitee_id` bigint NOT NULL,
  `status` tinyint NOT NULL DEFAULT 0 COMMENT '0=pending 1=accepted 2=declined 3=answered 4=revoked 5=expired',
  `active_key` bigint GENERATED ALWAYS AS (IF(status IN (0, 1), 1, invitation_id)) STORED COMMENT '有效邀请唯一键：有效行=1，终态行=invitation_id',
  `request_id` varchar(64) NOT NULL DEFAULT '' COMMENT '请求追踪/幂等关联',
  `expires_at` datetime NOT NULL COMMENT '默认发送后 7 天',
  `responded_at` datetime NULL,
  `answered_at` datetime NULL,
  `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_id` (`invitation_id`),
  UNIQUE KEY `uk_question_invitee_active` (`question_id`, `invitee_id`, `active_key`),
  KEY `idx_invitee_status` (`invitee_id`, `status`, `create_time`),
  KEY `idx_question_status` (`question_id`, `status`, `create_time`),
  KEY `idx_inviter_time` (`inviter_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 邀请接收偏好（PRD §9）：关闭后不出现在候选、不能收到新邀请；不影响已接受邀请的回答入口
ALTER TABLE `user`
  ADD COLUMN `answer_invite_enabled` tinyint NOT NULL DEFAULT 1 COMMENT '0=不接收回答邀请 1=接收';
