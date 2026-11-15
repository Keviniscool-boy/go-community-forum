CREATE TABLE `user` (

    `id` bigint(20) NOT NULL AUTO_INCREMENT,
    `username` varchar(64) COLLATE utf8mb4_general_ci NOT NULL ,
    `email` varchar(255) COLLATE utf8mb4_general_ci NOT NULL,
    `email_verified` tinyint(4) NOT NULL DEFAULT 0 COMMENT '邮箱是否已验证',
    `password` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
    `user_id` bigint(20) NOT NULL,
    `gender` tinyint(4) NOT NULL DEFAULT 0,
    `avatar_url` varchar(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '头像地址',
    `bio` varchar(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '个人简介',
    `status` tinyint(4) NOT NULL DEFAULT 1 COMMENT '账号状态 1 正常 0 禁用',
    `last_login_at` timestamp NULL DEFAULT NULL COMMENT '最后登录时间',
    `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
    `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_username` (`username`) USING BTREE,
    UNIQUE KEY `idx_user_id` (`user_id`) USING BTREE,
    UNIQUE KEY `uk_user_email` (`email`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `community` (
    `id` int(11) NOT NULL AUTO_INCREMENT, 
    `community_id` int(10) unsigned NOT NULL, 
    `community_name` varchar(20) COLLATE utf8mb4_general_ci NOT NULL,
    `introduction` varchar(256) COLLATE utf8mb4_general_ci NOT NULL, 
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_community_id` (`community_id`),
    UNIQUE KEY `idx_community_name` (`community_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

INSERT INTO `community` (`community_id`,`community_name`,`introduction`,`create_time`,`update_time`)
VALUES
  (1, 'GO', 'golang', '2026-06-01 06:03:00', '2026-06-01 06:03:00'),
  (2, 'JAVA', 'java', '2026-06-01 06:03:00', '2026-06-01 06:03:00'),
  (3, 'PYTHON', 'python', '2026-06-01 06:03:00', '2026-06-01 06:03:00'),
  (4, 'C++', 'c++', '2026-06-01 06:03:00', '2026-06-01 06:03:00');

DROP TABLE IF EXISTS `post`;
CREATE TABLE `post` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `post_id` bigint(20) NOT NULL COMMENT '帖子id',
  `title` varchar(128) COLLATE utf8mb4_general_ci NOT NULL COMMENT '标题',
  `content` varchar(8192) COLLATE utf8mb4_general_ci NOT NULL COMMENT '内容',
  `author_id` bigint(20) NOT NULL COMMENT '作者的user_id',
  `community_id` bigint(20) NOT NULL COMMENT '所属社区',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '帖子状态',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_post_id` (`post_id`),
  KEY `idx_author_id` (`author_id`),
  KEY `idx_community_id` (`community_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

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
