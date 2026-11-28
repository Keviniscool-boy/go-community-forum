-- 008_comment_enhance.sql 第四阶段：评论体系补全（点赞、楼中楼、删除）
-- 历史数据默认策略（明确决策）：存量评论的点赞数从 0 起算（此前无点赞功能）；
-- 楼中楼统一为两级结构（一级评论 + 其下回复），存量更深层的回复上提到所属一级评论下，
-- 并按回填后的回复关系计算 reply_count，保证升级后「查看 N 条回复」与列表一致。
-- 回滚说明：DROP TABLE comment_like；ALTER TABLE comment DROP COLUMN like_count, reply_count,
-- reply_to_user_id，DROP KEY idx_comment_parent（软删除复用 006 的 status / deleted_at，无新增删除列）。

ALTER TABLE `comment`
  ADD COLUMN `like_count` int(11) NOT NULL DEFAULT 0 COMMENT '点赞数（以 comment_like 关系表为准重算）',
  ADD COLUMN `reply_count` int(11) NOT NULL DEFAULT 0 COMMENT '直接回复数（仅可见回复）',
  ADD COLUMN `reply_to_user_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '被回复的回复的作者（楼中楼里「回复 @xxx」），0 表示直接回复一级评论',
  ADD KEY `idx_comment_parent` (`parent_id`, `create_time`);

CREATE TABLE IF NOT EXISTS `comment_like` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `comment_id` bigint(20) NOT NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_comment_like` (`user_id`, `comment_id`),
  KEY `idx_comment_like_comment` (`comment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 存量数据里直接挂在「回复」下的评论：上提到所属一级评论下，并记录被回复人
UPDATE `comment` r
  JOIN `comment` p ON p.comment_id = r.parent_id AND p.parent_id <> 0
  SET r.parent_id = p.parent_id, r.reply_to_user_id = p.user_id;

-- 同表回填用派生表连接（MySQL 不允许 UPDATE 的子查询直接引用目标表）
UPDATE `comment` c
  LEFT JOIN (
    SELECT parent_id, COUNT(*) AS cnt
    FROM `comment`
    WHERE parent_id <> 0 AND status = 1 AND deleted_at IS NULL
    GROUP BY parent_id
  ) r ON r.parent_id = c.comment_id
  SET c.reply_count = COALESCE(r.cnt, 0);
