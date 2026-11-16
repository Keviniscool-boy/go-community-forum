-- 009_growth_personalization.sql PRD1：专栏关注、兴趣、推荐偏好、负反馈、屏蔽、申诉
-- 所有关系表联合唯一保证幂等；fail-closed 审核与治理字段沿用 006 模型

CREATE TABLE IF NOT EXISTS `column_follow` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `column_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_column_follow` (`user_id`, `column_id`),
  KEY `idx_column_follow_column` (`column_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

ALTER TABLE `column_info`
  ADD COLUMN `follower_count` int(11) NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS `user_topic_interest` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `topic_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_topic_interest` (`user_id`, `topic_id`),
  KEY `idx_interest_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `recommendation_preference` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `personalization_enabled` tinyint(4) NOT NULL DEFAULT 1,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '乐观锁',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pref_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `recommendation_feedback` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'not_interested | reduce_topic',
  `target_type` varchar(16) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'content | topic',
  `target_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_feedback` (`user_id`, `kind`, `target_type`, `target_id`),
  KEY `idx_feedback_user` (`user_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `user_block` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `blocker_id` bigint(20) NOT NULL,
  `blocked_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_block` (`blocker_id`, `blocked_id`),
  KEY `idx_block_blocked` (`blocked_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `moderation_appeal` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `appeal_id` bigint(20) NOT NULL,
  `author_id` bigint(20) NOT NULL COMMENT '内容作者（申诉人）',
  `target_type` tinyint(4) NOT NULL COMMENT '1 question 2 answer 3 article',
  `target_id` bigint(20) NOT NULL,
  `decision_version` bigint(20) NOT NULL COMMENT '申诉针对的审核决定版本',
  `reason` varchar(1000) COLLATE utf8mb4_general_ci NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 pending 1 upheld(恢复) 2 rejected(维持)',
  `reviewer_id` bigint(20) NULL DEFAULT NULL,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '乐观锁',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `reviewed_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_appeal_id` (`appeal_id`),
  UNIQUE KEY `uk_appeal_decision` (`author_id`, `target_type`, `target_id`, `decision_version`, `status`, `create_time`),
  KEY `idx_appeal_queue` (`status`, `create_time`),
  KEY `idx_appeal_target` (`target_type`, `target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- 同一审核决定最多一个待处理申诉：应用层事务校验（status pending 且同 target 同 decision_version 查重）
