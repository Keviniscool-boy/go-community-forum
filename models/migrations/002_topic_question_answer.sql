-- 002_topic_question_answer.sql 话题、问题、回答表
-- 顺带建 question <-> topic 的关联表（提问时可带 topic_ids）

CREATE TABLE IF NOT EXISTS `topic` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `topic_id` bigint(20) NOT NULL COMMENT '话题id（雪花id）',
  `name` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `description` varchar(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `avatar_url` varchar(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `follower_count` int(11) NOT NULL DEFAULT 0,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_topic_id` (`topic_id`),
  UNIQUE KEY `uk_topic_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `question` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `question_id` bigint(20) NOT NULL COMMENT '问题id（雪花id）',
  `title` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `description` text COLLATE utf8mb4_general_ci NOT NULL,
  `author_id` bigint(20) NOT NULL COMMENT '作者的user_id',
  `answer_count` int(11) NOT NULL DEFAULT 0,
  `follower_count` int(11) NOT NULL DEFAULT 0,
  `view_count` int(11) NOT NULL DEFAULT 0,
  `status` tinyint(4) NOT NULL DEFAULT 1,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_question_id` (`question_id`),
  KEY `idx_question_author_time` (`author_id`, `create_time`),
  KEY `idx_question_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `answer` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `answer_id` bigint(20) NOT NULL COMMENT '回答id（雪花id）',
  `question_id` bigint(20) NOT NULL COMMENT '所属问题id',
  `author_id` bigint(20) NOT NULL COMMENT '作者的user_id',
  `content` mediumtext COLLATE utf8mb4_general_ci NOT NULL,
  `vote_count` int(11) NOT NULL DEFAULT 0,
  `comment_count` int(11) NOT NULL DEFAULT 0,
  `collect_count` int(11) NOT NULL DEFAULT 0,
  `status` tinyint(4) NOT NULL DEFAULT 1,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_answer_id` (`answer_id`),
  KEY `idx_answer_question_time` (`question_id`, `create_time`),
  KEY `idx_answer_author_time` (`author_id`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `question_topic` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `question_id` bigint(20) NOT NULL,
  `topic_id` bigint(20) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_question_topic` (`question_id`, `topic_id`),
  KEY `idx_question_topic_topic` (`topic_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
