package mysql

import (
	"sync"
	"testing"

	"bluebell/models"
)

// TestInteractionConsistency 互动一致性：并发幂等、通知同事务、计数不漂移（需真实 MySQL）
func TestInteractionConsistency(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "interact")
	switchToTempDB(t, fullName)

	// 建最小表结构（与真实迁移一致的关键列）
	mustExec(t, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, email_verified tinyint NOT NULL DEFAULT 0, password varchar(64) NOT NULL,
		avatar_url varchar(500) NOT NULL DEFAULT '', bio varchar(500) NOT NULL DEFAULT '',
		status tinyint NOT NULL DEFAULT 1, token_version int NOT NULL DEFAULT 0,
		PRIMARY KEY (id), UNIQUE KEY idx_user_id (user_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE question (
		id bigint NOT NULL AUTO_INCREMENT, question_id bigint NOT NULL, title varchar(200) NOT NULL,
		description text NOT NULL, author_id bigint NOT NULL, answer_count int NOT NULL DEFAULT 0,
		comment_count int NOT NULL DEFAULT 0, follower_count int NOT NULL DEFAULT 0,
		view_count int NOT NULL DEFAULT 0, status tinyint NOT NULL DEFAULT 1,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_question_id (question_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE answer (
		id bigint NOT NULL AUTO_INCREMENT, answer_id bigint NOT NULL, question_id bigint NOT NULL,
		author_id bigint NOT NULL, content mediumtext NOT NULL, vote_count int NOT NULL DEFAULT 0,
		comment_count int NOT NULL DEFAULT 0, collect_count int NOT NULL DEFAULT 0,
		status tinyint NOT NULL DEFAULT 1, create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_answer_id (answer_id)) ENGINE=InnoDB`)
	// 互动关系表（与迁移 003 / 008 一致的最小结构）
	mustExec(t, `CREATE TABLE user_follow (
		id bigint NOT NULL AUTO_INCREMENT, follower_id bigint NOT NULL, following_id bigint NOT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_follower_following (follower_id, following_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE question_follow (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, question_id bigint NOT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_question_follow (user_id, question_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE answer_vote (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, answer_id bigint NOT NULL,
		direction tinyint NOT NULL DEFAULT 1, create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		update_time timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_answer_vote (user_id, answer_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE answer_collect (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, answer_id bigint NOT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_answer_collect (user_id, answer_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE comment (
		id bigint NOT NULL AUTO_INCREMENT, comment_id bigint NOT NULL, user_id bigint NOT NULL,
		target_type tinyint NOT NULL, target_id bigint NOT NULL, parent_id bigint NOT NULL DEFAULT 0,
		reply_to_user_id bigint NOT NULL DEFAULT 0,
		content varchar(2000) NOT NULL, status tinyint NOT NULL DEFAULT 1,
		moderation_status tinyint NOT NULL DEFAULT 1, deleted_at timestamp NULL DEFAULT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_comment_id (comment_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE notification (
		id bigint NOT NULL AUTO_INCREMENT, notification_id bigint NOT NULL, user_id bigint NOT NULL,
		actor_id bigint NOT NULL, type tinyint NOT NULL, target_type tinyint NOT NULL,
		target_id bigint NOT NULL, is_read tinyint NOT NULL DEFAULT 0,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_notification_id (notification_id)) ENGINE=InnoDB`)

	// 用户与内容
	mustExec(t, "INSERT INTO user (user_id, username, email, password) VALUES (1, 'author', 'a@t.local', 'x'), (2, 'voter', 'b@t.local', 'x')")
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id) VALUES (10, 'q', 'd', 1)")
	mustExec(t, "INSERT INTO answer (answer_id, question_id, author_id, content) VALUES (20, 10, 1, 'ans')")

	// 并发 20 个 goroutine 同时投票赞同 + 关注问题 + 收藏回答
	var wg sync.WaitGroup
	errCh := make(chan error, 60)
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); if err := VoteAnswer(2, 20, 1); err != nil { errCh <- err } }()
		go func() { defer wg.Done(); if err := FollowQuestion(2, 10); err != nil { errCh <- err } }()
		go func() { defer wg.Done(); if err := CollectAnswer(2, 20); err != nil { errCh <- err } }()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent interaction failed: %v", err)
	}

	// 关系表只有一条；计数精确为 1；非负
	var votes, follows, collects int
	mustQueryRow(t, db, "SELECT COUNT(*) FROM answer_vote WHERE user_id=2 AND answer_id=20", &votes)
	mustQueryRow(t, db, "SELECT COUNT(*) FROM question_follow WHERE user_id=2 AND question_id=10", &follows)
	mustQueryRow(t, db, "SELECT COUNT(*) FROM answer_collect WHERE user_id=2 AND answer_id=20", &collects)
	if votes != 1 || follows != 1 || collects != 1 {
		t.Fatalf("duplicate relations: votes=%d follows=%d collects=%d", votes, follows, collects)
	}

	var voteCount, collectCount, followerCount, commentCount int
	mustQueryRow(t, db, "SELECT vote_count FROM answer WHERE answer_id=20", &voteCount)
	mustQueryRow(t, db, "SELECT collect_count FROM answer WHERE answer_id=20", &collectCount)
	mustQueryRow(t, db, "SELECT follower_count FROM question WHERE question_id=10", &followerCount)
	if voteCount != 1 || collectCount != 1 || followerCount != 1 {
		t.Fatalf("counters wrong: vote=%d collect=%d follower=%d", voteCount, collectCount, followerCount)
	}

	// 取消后重复取消：计数回到 0，不为负
	if err := UncollectAnswer(2, 20); err != nil {
		t.Fatal(err)
	}
	if err := UncollectAnswer(2, 20); err != nil {
		t.Fatal(err)
	}
	mustQueryRow(t, db, "SELECT collect_count FROM answer WHERE answer_id=20", &collectCount)
	if collectCount != 0 {
		t.Fatalf("collect_count should be 0 after double uncollect, got %d", collectCount)
	}

	// 评论：计数 +1，通知同事务写入
	cm := &models.Comment{
		CommentID:  300,
		UserID:     2,
		TargetType: models.TargetAnswer,
		TargetID:   20,
		Content:    "学习了",
	}
	if err := CreateComment(cm); err != nil {
		t.Fatal(err)
	}
	mustQueryRow(t, db, "SELECT comment_count FROM answer WHERE answer_id=20", &commentCount)
	if commentCount != 1 {
		t.Fatalf("comment_count=%d", commentCount)
	}
	var notifCount int
	mustQueryRow(t, db, "SELECT COUNT(*) FROM notification WHERE user_id=1 AND actor_id=2 AND type=2", &notifCount)
	if notifCount != 1 {
		t.Fatalf("comment notification missing: %d", notifCount)
	}
}
