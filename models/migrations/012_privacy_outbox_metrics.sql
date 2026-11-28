-- 012_privacy_outbox_metrics.sql PRD1/PRD2 缺口补齐：
-- 消息举报证据（加密快照）、事务性 outbox、内容逐日指标聚合、账号删除去标识化

CREATE TABLE IF NOT EXISTS `message_report_evidence` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `report_id` bigint(20) NOT NULL COMMENT '关联 report.report_id',
  `message_id` bigint(20) NOT NULL,
  `encrypted_snapshot` varbinary(4096) NOT NULL COMMENT 'AES-256-GCM 加密的消息快照',
  `key_version` int(11) NOT NULL DEFAULT 1 COMMENT '加密密钥版本（密钥与数据库分离管理）',
  `retain_until` timestamp NOT NULL COMMENT '证据保留截止（举报处理完成 + 保留期）',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_evidence_report` (`report_id`),
  KEY `idx_evidence_retain` (`retain_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `outbox` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `event_id` bigint(20) NOT NULL COMMENT '幂等键',
  `event_type` varchar(32) COLLATE utf8mb4_general_ci NOT NULL,
  `aggregate_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `payload` json NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 pending 1 done 2 dead',
  `attempts` int(11) NOT NULL DEFAULT 0,
  `next_attempt_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `last_error` varchar(500) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_outbox_event` (`event_id`),
  KEY `idx_outbox_dispatch` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `content_daily_metric` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL COMMENT 'UTC 日',
  `content_type` varchar(16) COLLATE utf8mb4_general_ci NOT NULL,
  `content_id` bigint(20) NOT NULL,
  `metric_version` int(11) NOT NULL DEFAULT 1,
  `impressions` bigint(20) NOT NULL DEFAULT 0,
  `clicks` bigint(20) NOT NULL DEFAULT 0,
  `views` bigint(20) NOT NULL DEFAULT 0,
  `computed_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_daily_metric` (`date`, `content_type`, `content_id`, `metric_version`),
  KEY `idx_metric_content` (`content_type`, `content_id`, `date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 账号删除去标识化（DELETE /me/account 使用）：把 PII 字段替换为不可逆占位，
-- 内容保留但作者匿名（用户请求删除/去标识化，规格 9.5）
UPDATE `user` SET `status` = 0 WHERE 1 = 0; -- 占位语句保持脚本结构（实际删除在应用层事务）
