-- 008_articles_columns.sql 前置阶段：文章与专栏实体
-- 决策记录（PRD §12.1 默认）：文章允许无专栏（column_id 可空，索引按需查询）
-- 治理字段沿用 006 模型：moderation_status/visibility/deleted_at；发布走 fail-closed 敏感词检查
-- 存量数据：新表无存量，默认 approved（列默认值）

CREATE TABLE IF NOT EXISTS `article` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `article_id` bigint(20) NOT NULL COMMENT '文章id（雪花）',
  `author_id` bigint(20) NOT NULL,
  `column_id` bigint(20) NULL DEFAULT NULL COMMENT '所属专栏，NULL 表示无专栏',
  `title` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `content` mediumtext COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Markdown 源文本（安全子集）',
  `view_count` int(11) NOT NULL DEFAULT 0,
  `status` tinyint(4) NOT NULL DEFAULT 1,
  `moderation_status` tinyint(4) NOT NULL DEFAULT 1,
  `visibility` tinyint(4) NOT NULL DEFAULT 1,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `updated_by` bigint(20) NULL DEFAULT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_article_id` (`article_id`),
  KEY `idx_article_author_time` (`author_id`, `status`, `create_time`),
  KEY `idx_article_column` (`column_id`, `status`),
  KEY `idx_article_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `column_info` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `column_id` bigint(20) NOT NULL COMMENT '专栏id（雪花）',
  `owner_id` bigint(20) NOT NULL COMMENT '所有者',
  `name` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `description` varchar(1000) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `status` tinyint(4) NOT NULL DEFAULT 1,
  `moderation_status` tinyint(4) NOT NULL DEFAULT 1,
  `visibility` tinyint(4) NOT NULL DEFAULT 1,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `updated_by` bigint(20) NULL DEFAULT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_column_id` (`column_id`),
  KEY `idx_column_owner` (`owner_id`, `status`),
  KEY `idx_column_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- 表名 column_info：column 是 MySQL 保留字风险名，遵循现有库规范避免歧义

-- 评论目标类型扩展：文章 = 3（复用 comment 表，target_type 增加 3）
-- 修改 comment 的目标索引覆盖（已有 idx_comment_target 按类型查询，无需变更）

-- 草稿类型扩展：article = 2（draft 表 draft_type 为 tinyint，直接使用）
