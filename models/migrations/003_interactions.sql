-- 003_interactions.sql 关注、点赞、收藏、评论、通知
-- 所有关系表使用联合唯一索引保证幂等（重复关注/点赞/收藏不会产生重复记录）

CREATE TABLE IF NOT EXISTS `user_follow` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `follower_id` bigint(20) NOT NULL COMMENT '发起关注的人',
  `following_id` bigint(20) NOT NULL COMMENT '被关注的人',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_follower_following` (`follower_id`, `following_id`),
  KEY `idx_user_follow_following` (`following_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `topic_follow` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `topic_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_topic_follow` (`user_id`, `topic_id`),
  KEY `idx_topic_follow_topic` (`topic_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `question_follow` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `question_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_question_follow` (`user_id`, `question_id`),
  KEY `idx_question_follow_question` (`question_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `answer_vote` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `answer_id` bigint(20) NOT NULL,
  `direction` tinyint(4) NOT NULL DEFAULT 1 COMMENT '1 赞同 -1 反对',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_answer_vote` (`user_id`, `answer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `answer_collect` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `answer_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_answer_collect` (`user_id`, `answer_id`),
  KEY `idx_answer_collect_answer` (`answer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `comment` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `comment_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL COMMENT '评论者',
  `target_type` tinyint(4) NOT NULL COMMENT '1 回答 2 问题',
  `target_id` bigint(20) NOT NULL,
  `parent_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '父评论 id，0 表示一级评论',
  `content` varchar(2000) COLLATE utf8mb4_general_ci NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT 1,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_comment_id` (`comment_id`),
  KEY `idx_comment_target` (`target_type`, `target_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `notification` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `notification_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL COMMENT '接收通知的人',
  `actor_id` bigint(20) NOT NULL COMMENT '触发通知的人',
  `type` tinyint(4) NOT NULL COMMENT '1 赞同 2 评论 3 关注 4 收藏',
  `target_type` tinyint(4) NOT NULL,
  `target_id` bigint(20) NOT NULL,
  `is_read` tinyint(4) NOT NULL DEFAULT 0,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_id` (`notification_id`),
  KEY `idx_notification_user` (`user_id`, `is_read`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 问题表补充评论数列
ALTER TABLE `question`
  ADD COLUMN `comment_count` int(11) NOT NULL DEFAULT 0 COMMENT '评论数';
