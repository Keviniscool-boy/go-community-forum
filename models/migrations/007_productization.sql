-- 007_productization.sql 第三阶段 P1/P2：敏感词、草稿、媒体、收藏夹、历史、推荐事件、系统配置

CREATE TABLE IF NOT EXISTS `sensitive_word` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `word` varchar(128) COLLATE utf8mb4_general_ci NOT NULL,
  `word_type` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 精确匹配 1 审核拒绝词（命中直接 pending）',
  `enabled` tinyint(4) NOT NULL DEFAULT 1,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '词表版本，更新时递增触发缓存失效',
  `created_by` bigint(20) NULL DEFAULT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_word` (`word`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `draft` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `draft_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `draft_type` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 question 1 answer（answer 关联 question_id）',
  `question_id` bigint(20) NOT NULL DEFAULT 0,
  `title` varchar(200) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `content` mediumtext COLLATE utf8mb4_general_ci NOT NULL,
  `version` bigint(20) NOT NULL DEFAULT 1 COMMENT '自动保存冲突检测',
  `deleted_at` timestamp NULL DEFAULT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_draft_id` (`draft_id`),
  KEY `idx_draft_user` (`user_id`, `draft_type`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `media_asset` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `asset_id` bigint(20) NOT NULL,
  `owner_id` bigint(20) NOT NULL,
  `object_key` varchar(500) COLLATE utf8mb4_general_ci NOT NULL,
  `mime_type` varchar(100) COLLATE utf8mb4_general_ci NOT NULL,
  `byte_size` bigint(20) NOT NULL,
  `sha256` char(64) COLLATE utf8mb4_general_ci NOT NULL,
  `width` int(11) NULL DEFAULT NULL,
  `height` int(11) NULL DEFAULT NULL,
  `status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 uploading 1 ready 2 rejected 3 deleted',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `deleted_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_asset_id` (`asset_id`),
  UNIQUE KEY `uk_asset_hash_owner` (`owner_id`, `sha256`),
  KEY `idx_asset_owner_time` (`owner_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `bookmark_folder` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `folder_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `name` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `is_private` tinyint(4) NOT NULL DEFAULT 1,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_folder_id` (`folder_id`),
  UNIQUE KEY `uk_user_folder_name` (`user_id`, `name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `bookmark_item` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `folder_id` bigint(20) NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `target_type` tinyint(4) NOT NULL,
  `target_id` bigint(20) NOT NULL,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_folder_target` (`folder_id`, `target_type`, `target_id`),
  KEY `idx_user_bookmark_time` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `content_view_history` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `target_type` tinyint(4) NOT NULL,
  `target_id` bigint(20) NOT NULL,
  `last_viewed_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `view_count` int(11) NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_target` (`user_id`, `target_type`, `target_id`),
  KEY `idx_user_history` (`user_id`, `last_viewed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `recommendation_event` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `event_id` bigint(20) NOT NULL,
  `anonymous_id_hash` char(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `user_id` bigint(20) NULL DEFAULT NULL,
  `session_id_hash` char(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `event_type` varchar(32) COLLATE utf8mb4_general_ci NOT NULL,
  `item_type` varchar(32) COLLATE utf8mb4_general_ci NOT NULL,
  `item_id` bigint(20) NOT NULL,
  `feed_type` varchar(32) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `position` int(11) NULL DEFAULT NULL,
  `request_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `occurred_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `metadata_json` json NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_event_id` (`event_id`),
  KEY `idx_event_time_type` (`occurred_at`, `event_type`),
  KEY `idx_event_item` (`item_type`, `item_id`, `occurred_at`),
  KEY `idx_event_user_time` (`user_id`, `occurred_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `system_setting` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `setting_key` varchar(128) COLLATE utf8mb4_general_ci NOT NULL,
  `setting_value` text COLLATE utf8mb4_general_ci NOT NULL,
  `value_type` varchar(16) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'string',
  `is_secret` tinyint(4) NOT NULL DEFAULT 0,
  `version` bigint(20) NOT NULL DEFAULT 1,
  `updated_by` bigint(20) NULL DEFAULT NULL,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_setting_key` (`setting_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
