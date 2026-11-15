package mysql

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/models"

	"github.com/go-sql-driver/mysql"

	"go.uber.org/zap"
)

var (
	ErrTopicNotExist = errors.New("话题不存在")
	ErrTopicExist    = errors.New("话题已存在")
)

// GetTopicsByQuestionIDs 批量查询多个问题的话题（返回 questionID -> 话题列表）
func GetTopicsByQuestionIDs(questionIDs []int64) (map[int64][]*models.TopicBrief, error) {
	result := make(map[int64][]*models.TopicBrief)
	if len(questionIDs) == 0 {
		return result, nil
	}
	placeholders := ""
	for i := range questionIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	query := `SELECT qt.question_id, t.topic_id, t.name
		  FROM question_topic qt
		  JOIN topic t ON t.topic_id = qt.topic_id
		  WHERE qt.question_id IN (` + placeholders + `)`
	args := make([]interface{}, 0, len(questionIDs))
	for _, id := range questionIDs {
		args = append(args, id)
	}
	type row struct {
		QuestionID int64  `db:"question_id"`
		TopicID    int64  `db:"topic_id"`
		Name       string `db:"name"`
	}
	rows := make([]row, 0)
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.QuestionID] = append(result[r.QuestionID], &models.TopicBrief{
			TopicID: strconv.FormatInt(r.TopicID, 10),
			Name:    r.Name,
		})
	}
	return result, nil
}

// TopicExists 话题是否存在
func TopicExists(topicID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(topic_id) FROM topic WHERE topic_id = ?", topicID)
	return count > 0, err
}

// CreateTopic 创建话题
func CreateTopic(t *models.Topic) error {
	_, err := db.Exec("INSERT INTO topic (topic_id, name, description, avatar_url) VALUES (?, ?, ?, ?)",
		t.TopicID, t.Name, t.Description, t.AvatarURL)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrTopicExist
		}
	}
	return err
}

// ListTopics 话题列表（按关注人数倒序）
func ListTopics(limit int) (topics []*models.Topic, err error) {
	topics = make([]*models.Topic, 0, limit)
	err = db.Select(&topics, "SELECT topic_id, name, description, avatar_url, follower_count FROM topic ORDER BY follower_count DESC LIMIT ?", limit)
	return
}

// GetTopicByID 话题详情
func GetTopicByID(id int64) (t *models.Topic, err error) {
	t = new(models.Topic)
	err = db.Get(t, "SELECT topic_id, name, description, avatar_url, follower_count FROM topic WHERE topic_id = ?", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTopicNotExist
		}
		return nil, err
	}
	return t, nil
}

// GetQuestionsByTopicID 查询话题下的问题（最新优先）
func GetQuestionsByTopicID(topicID int64, limit int) (questions []*models.Question, err error) {
	questions = make([]*models.Question, 0, limit)
	err = db.Select(&questions, `SELECT q.question_id, q.title, q.description, q.author_id, q.answer_count,
		q.comment_count, q.follower_count, q.view_count, q.status, q.create_time
		FROM question_topic qt JOIN question q ON q.question_id = qt.question_id
		WHERE qt.topic_id = ? AND q.status = 1
		ORDER BY q.create_time DESC LIMIT ?`, topicID, limit)
	if err != nil {
		zap.L().Error("query topic questions failed", zap.Error(err))
		return nil, err
	}
	return
}

// FollowTopic 关注话题（幂等）
func FollowTopic(userID, topicID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT IGNORE INTO topic_follow (user_id, topic_id) VALUES (?, ?)", userID, topicID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE topic SET follower_count = follower_count + 1 WHERE topic_id = ?", topicID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UnfollowTopic 取消关注话题
func UnfollowTopic(userID, topicID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("DELETE FROM topic_follow WHERE user_id = ? AND topic_id = ?", userID, topicID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE topic SET follower_count = follower_count - 1 WHERE topic_id = ?", topicID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsTopicFollowed 查询话题关注状态
func IsTopicFollowed(userID, topicID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM topic_follow WHERE user_id = ? AND topic_id = ?", userID, topicID)
	return count > 0, err
}
