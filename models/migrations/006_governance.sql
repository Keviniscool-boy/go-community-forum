-- 006_governance.sql 第三阶段：内容治理字段、用户角色、举报与审计日志
-- 历史数据默认策略（明确决策）：存量已发布内容直接标记为已审核（moderation_status=1），
-- 避免存量内容全部消失；第三阶段起新创建内容走敏感词检查 + 审核状态机。
-- 回滚说明：治理字段为附加列，删除即可；report/audit_log 为新表，DROP 即可。

ALTER TABLE `user`
  ADD COLUMN `role` VARCHAR(16) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'user' COMMENT 'user | moderator | admin';

ALTER TABLE `question`
  ADD COLUMN `moderation_status` TINYINT NOT NULL DEFAULT 1 COMMENT '0 pending 1 approved 2 rejected 3 quarantined',
  ADD COLUMN `visibility` TINYINT NOT NULL DEFAULT 1 COMMENT '0 hidden 1 public 2 author_only',
  ADD COLUMN `deleted_at` TIMESTAMP NULL DEFAULT NULL,
  ADD COLUMN `updated_by` BIGINT NULL DEFAULT NULL;

ALTER TABLE `answer`
  ADD COLUMN `moderation_status` TINYINT NOT NULL DEFAULT 1,
  ADD COLUMN `visibility` TINYINT NOT NULL DEFAULT 1,
  ADD COLUMN `deleted_at` TIMESTAMP NULL DEFAULT NULL,
  ADD COLUMN `updated_by` BIGINT NULL DEFAULT NULL;

ALTER TABLE `comment`
  ADD COLUMN `moderation_status` TINYINT NOT NULL DEFAULT 1,
  ADD COLUMN `deleted_at` TIMESTAMP NULL DEFAULT NULL,
  ADD COLUMN `updated_by` BIGINT NULL DEFAULT NULL;

-- 历史内容标记为已审核（默认策略，见文件头注释）
UPDATE `question` SET moderation_status = 1 WHERE status = 1;
UPDATE `answer` SET moderation_status = 1 WHERE status = 1;
UPDATE `comment` SET moderation_status = 1 WHERE status = 1;

CREATE TABLE IF NOT EXISTS `report` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `report_id` bigint(20) NOT NULL,
  `reporter_id` bigint(20) NOT NULL,
  `target_type` tinyint(4) NOT NULL COMMENT '1 question 2 answer 3 comment 4 user',
  `target_id` bigint(20) NOT NULL,
  `reason_code` varchar(32) COLLATE utf8mb4_general_ci NOT NULL,
  `description` varchar(1000) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `status` tinyint(4) NOT NULL DEFAULT 0 COMMENT '0 pending 1 resolved 2 dismissed',
  `reviewer_id` bigint(20) NULL DEFAULT NULL,
  `review_note` varchar(1000) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `reviewed_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_report_id` (`report_id`),
  UNIQUE KEY `uk_reporter_target` (`reporter_id`, `target_type`, `target_id`),
  KEY `idx_report_queue` (`status`, `created_at`),
  KEY `idx_report_target` (`target_type`, `target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `audit_log` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `audit_id` bigint(20) NOT NULL,
  `actor_id` bigint(20) NULL DEFAULT NULL,
  `action` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `target_type` varchar(32) COLLATE utf8mb4_general_ci NOT NULL,
  `target_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `request_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `ip_hash` varchar(128) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '',
  `metadata_json` json NOT NULL,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_audit_id` (`audit_id`),
  KEY `idx_audit_actor_time` (`actor_id`, `created_at`),
  KEY `idx_audit_target_time` (`target_type`, `target_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
