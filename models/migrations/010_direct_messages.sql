-- 010_direct_messages.sql PRD2：私信——隐私设置、会话、成员、消息
-- 会话唯一标识：排序后的两个用户 ID（user_low_id < user_high_id），一个参与者对只有一个有效会话链
-- 默认隐私策略：仅允许我关注的人发起请求（following）；用户可改为 nobody/everyone

CREATE TABLE IF NOT EXISTS `message_privacy_setting` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `incoming_policy` varchar(16) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'following' COMMENT 'nobody | following | everyone',
  `read_receipts_enabled` tinyint(4) NOT NULL DEFAULT 1,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '乐观锁',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_privacy_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `conversation` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `conversation_id` bigint(20) NOT NULL COMMENT '会话id（雪花）',
  `user_low_id` bigint(20) NOT NULL,
  `user_high_id` bigint(20) NOT NULL,
  `initiator_id` bigint(20) NOT NULL COMMENT '发起请求的人',
  `state` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 pending 1 accepted 2 declined 3 closed',
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '状态乐观锁',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_conversation_id` (`conversation_id`),
  UNIQUE KEY `uk_pair` (`user_low_id`, `user_high_id`),
  KEY `idx_conv_pair_state` (`user_low_id`, `user_high_id`, `state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `conversation_member` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `conversation_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `last_read_message_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '已读水位（单调推进）',
  `hidden_at` timestamp NULL DEFAULT NULL COMMENT '本人隐藏会话时间',
  `closed_at` timestamp NULL DEFAULT NULL COMMENT '本人关闭会话时间',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_conv_member` (`conversation_id`, `user_id`),
  KEY `idx_member_user` (`user_id`, `conversation_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `direct_message` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `message_id` bigint(20) NOT NULL,
  `conversation_id` bigint(20) NOT NULL,
  `sender_id` bigint(20) NOT NULL,
  `client_message_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL COMMENT '客户端幂等 ID',
  `body` varchar(2000) COLLATE utf8mb4_general_ci NOT NULL COMMENT '纯文本，1-2000 Unicode 字符',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_message_id` (`message_id`),
  UNIQUE KEY `uk_client_msg` (`conversation_id`, `sender_id`, `client_message_id`),
  KEY `idx_msg_conv_time` (`conversation_id`, `create_time`, `message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
