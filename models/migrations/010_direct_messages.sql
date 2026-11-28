-- 010_direct_messages.sql 第五阶段 M2（私信，PRD F6）
-- 一对一会话：user_a < user_b 唯一对；删除会话仅自己视角隐藏（a/b_deleted_at），消息保留。
-- 回滚说明：DROP TABLE message, message_thread。

CREATE TABLE IF NOT EXISTS `message_thread` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `thread_id` bigint NOT NULL COMMENT '雪花id',
  `user_a` bigint NOT NULL COMMENT '较小 user_id',
  `user_b` bigint NOT NULL COMMENT '较大 user_id',
  `last_message_id` bigint NOT NULL DEFAULT 0,
  `last_message_at` timestamp NULL DEFAULT NULL,
  `a_read_at` timestamp NULL DEFAULT NULL,
  `b_read_at` timestamp NULL DEFAULT NULL,
  `a_deleted_at` timestamp NULL DEFAULT NULL COMMENT 'a 删除会话（仅自己视角隐藏）',
  `b_deleted_at` timestamp NULL DEFAULT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_thread_id` (`thread_id`),
  UNIQUE KEY `uk_thread_pair` (`user_a`, `user_b`),
  KEY `idx_thread_a` (`user_a`, `last_message_at`),
  KEY `idx_thread_b` (`user_b`, `last_message_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `message_id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `sender_id` bigint NOT NULL,
  `content` varchar(2000) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_message_id` (`message_id`),
  KEY `idx_message_thread_time` (`thread_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
