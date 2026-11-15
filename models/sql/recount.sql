-- recount.sql 计数核对与修复：以关系表为准重算全部冗余计数
-- 场景：异常中断导致计数漂移、Redis 缓存重建前的事实校准。
-- 只读核对（找出不一致的行）：
SELECT q.question_id,
       q.answer_count                                     AS stored_answers,
       (SELECT COUNT(*) FROM answer a WHERE a.question_id = q.question_id AND a.status = 1) AS real_answers
FROM question q
HAVING stored_answers <> real_answers;

SELECT q.question_id,
       q.comment_count                                    AS stored_comments,
       (SELECT COUNT(*) FROM comment c WHERE c.target_type = 2 AND c.target_id = q.question_id AND c.status = 1) AS real_comments
FROM question q
HAVING stored_comments <> real_comments;

SELECT a.answer_id,
       a.vote_count                                       AS stored_votes,
       COALESCE(SUM(CASE WHEN av.direction = 1 THEN 1 WHEN av.direction = -1 THEN -1 ELSE 0 END), 0) AS real_votes
FROM answer a LEFT JOIN answer_vote av ON av.answer_id = a.answer_id
GROUP BY a.answer_id
HAVING stored_votes <> real_votes;

SELECT a.answer_id,
       a.collect_count                                    AS stored_collects,
       (SELECT COUNT(*) FROM answer_collect ac WHERE ac.answer_id = a.answer_id) AS real_collects
FROM answer a
HAVING stored_collects <> real_collects;

SELECT q.question_id,
       q.follower_count                                   AS stored_followers,
       (SELECT COUNT(*) FROM question_follow qf WHERE qf.question_id = q.question_id) AS real_followers
FROM question q
HAVING stored_followers <> real_followers;

-- 修复（以关系表为准覆盖冗余计数）：
UPDATE question q
   SET q.answer_count = (SELECT COUNT(*) FROM answer a WHERE a.question_id = q.question_id AND a.status = 1),
       q.comment_count = (SELECT COUNT(*) FROM comment c WHERE c.target_type = 2 AND c.target_id = q.question_id AND c.status = 1),
       q.follower_count = (SELECT COUNT(*) FROM question_follow qf WHERE qf.question_id = q.question_id);

UPDATE answer a
   SET a.vote_count = COALESCE((SELECT SUM(CASE WHEN av.direction = 1 THEN 1 WHEN av.direction = -1 THEN -1 ELSE 0 END)
                                FROM answer_vote av WHERE av.answer_id = a.answer_id), 0),
       a.collect_count = (SELECT COUNT(*) FROM answer_collect ac WHERE ac.answer_id = a.answer_id);
