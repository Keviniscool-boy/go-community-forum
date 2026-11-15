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

