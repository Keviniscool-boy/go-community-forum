-- 011_collaboration.sql PRD3：系列、专栏成员与邀请、文章版本、审阅
-- 一个系列属于一个专栏；一篇文章可加入多个系列，同一系列内唯一
-- 编辑/审阅批准不等于安全审核通过；发布仍走 fail-closed 审核

CREATE TABLE IF NOT EXISTS `series` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `series_id` bigint(20) NOT NULL,
  `column_id` bigint(20) NOT NULL,
  `title` varchar(100) COLLATE utf8mb4_general_ci NOT NULL,
  `description` varchar(1000) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `status` tinyint(4) NOT NULL DEFAULT 1 COMMENT '1 公开 0 下架',
  `moderation_status` tinyint(4) NOT NULL DEFAULT 1,
  `visibility` tinyint(4) NOT NULL DEFAULT 1,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '目录排序乐观锁',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_series_id` (`series_id`),
  KEY `idx_series_column` (`column_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `series_item` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `series_id` bigint(20) NOT NULL,
  `article_id` bigint(20) NOT NULL,
  `position` int(11) NOT NULL DEFAULT 0,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_series_article` (`series_id`, `article_id`),
  UNIQUE KEY `uk_series_position` (`series_id`, `position`),
  KEY `idx_series_item_article` (`article_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `column_member` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `column_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `role` varchar(16) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'owner | editor | reviewer',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_column_member` (`column_id`, `user_id`),
  KEY `idx_column_member_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `column_invitation` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `invitation_id` bigint(20) NOT NULL,
  `column_id` bigint(20) NOT NULL,
  `inviter_id` bigint(20) NOT NULL,
  `invitee_id` bigint(20) NOT NULL,
  `role` varchar(16) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'editor',
  `status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 pending 1 accepted 2 declined 3 revoked',
  `expires_at` timestamp NOT NULL COMMENT '7 天有效',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `reviewed_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_id` (`invitation_id`),
  KEY `idx_invitation_invitee` (`invitee_id`, `status`),
  KEY `idx_invitation_column` (`column_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- 幂等：同 column+invitee+pending 由应用层事务查重；重复发送返回已有邀请

CREATE TABLE IF NOT EXISTS `article_revision` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `revision_id` bigint(20) NOT NULL,
  `article_id` bigint(20) NOT NULL,
  `revision_no` bigint(20) NOT NULL,
  `title` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `body` mediumtext COLLATE utf8mb4_general_ci NOT NULL,
  `author_id` bigint(20) NOT NULL COMMENT '修改人',
  `content_hash` char(64) COLLATE utf8mb4_general_ci NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_revision_no` (`article_id`, `revision_no`),
  UNIQUE KEY `uk_revision_id` (`revision_id`),
  KEY `idx_revision_article_time` (`article_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `review_request` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `review_id` bigint(20) NOT NULL,
  `article_id` bigint(20) NOT NULL,
  `revision_id` bigint(20) NOT NULL,
  `state` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 in_review 1 approved 2 changes_requested',
  `initiator_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `reviewed_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_review_id` (`review_id`),
  UNIQUE KEY `uk_review_pending` (`article_id`, `revision_id`, `state`, `create_time`),
  KEY `idx_review_article_state` (`article_id`, `state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `review_comment` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `comment_id` bigint(20) NOT NULL,
  `review_id` bigint(20) NOT NULL,
  `author_id` bigint(20) NOT NULL,
  `body` varchar(2000) COLLATE utf8mb4_general_ci NOT NULL COMMENT '纯文本，仅协作者可见',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_review_comment_id` (`comment_id`),
  KEY `idx_review_comment_review` (`review_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `column_audit` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `audit_id` bigint(20) NOT NULL,
  `actor_id` bigint(20) NOT NULL,
  `column_id` bigint(20) NOT NULL,
  `action` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `target_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `request_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_column_audit_id` (`audit_id`),
  KEY `idx_column_audit_column` (`column_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 文章协作审阅状态（0 none 1 in_review 2 approved 3 changes_requested）
-- 文章协作审阅状态
ALTER TABLE `article`
  ADD COLUMN `collab_state` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 none 1 in_review 2 approved 3 changes_requested',
  ADD COLUMN `approved_revision_id` bigint(20) NULL DEFAULT NULL;
