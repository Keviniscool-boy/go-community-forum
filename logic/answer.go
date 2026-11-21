package logic

import (
	"errors"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// 问题相关业务错误
var (
	ErrQuestionNotExist = errors.New("问题不存在")
	ErrQuestionInvalid  = errors.New("问题参数不合法")
)

// CreateQuestion 创建问题
func CreateQuestion(p *models.ParamCreateQuestion, authorID int64) (questionID int64, err error) {
	// 解析话题 id（前端传雪花 id 字符串），话题必须真实存在
	topicIDs := make([]int64, 0, len(p.TopicIDs))
	for _, s := range p.TopicIDs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return 0, ErrQuestionInvalid
		}
		exists, err := mysql.TopicExists(id)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, ErrQuestionInvalid
		}
		topicIDs = append(topicIDs, id)
	}
	q := &models.Question{
		ID:          snowflake.GenerateID(),
		Title:       p.Title,
		Description: p.Description,
		AuthorID:    authorID,
	}
	// 发布审核流（规格 7.1）：参数校验 -> 敏感词检查 -> 决定审核状态
	decision, err := CheckContentModeration(p.Title, p.Description)
	if err != nil {
		return 0, err
	}
	if err = mysql.CreateQuestion(q, topicIDs, decision.Status); err != nil {
		return 0, err
	}
	return q.ID, nil
}

// GetQuestionDetail 问题详情（含作者名、话题），同时自增浏览量
// viewerID 为当前登录用户（游客 0）；author_only 内容仅作者本人可见
func GetQuestionDetail(id int64, viewerID int64) (data *models.ApiQuestionDetail, err error) {
	q, err := mysql.GetQuestionPublicByID(id, viewerID)
	if err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	// 浏览量自增失败不影响详情返回
	if err := mysql.IncrementQuestionViewCount(id); err != nil {
		zap.L().Warn("increment question view_count failed", zap.Int64("id", id), zap.Error(err))
	}
	user, err := mysql.GetUserByID(q.AuthorID)
	if err != nil {
		return nil, err
	}
	topics, err := mysql.GetQuestionTopics(id)
	if err != nil {
		return nil, err
	}
	data = &models.ApiQuestionDetail{
		Question:   q,
		AuthorName: user.Username,
		Topics:     topics,
	}
	return data, nil
}

// ---------- 回答 ----------

// CreateAnswer 创建回答
func CreateAnswer(questionID int64, p *models.ParamCreateAnswer, authorID int64) (answerID int64, err error) {
	// 先确认问题存在
	if _, err = mysql.GetQuestionByID(questionID); err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return 0, ErrQuestionNotExist
		}
		return 0, err
	}
	a := &models.Answer{
		ID:         snowflake.GenerateID(),
		QuestionID: questionID,
		AuthorID:   authorID,
		Content:    p.Content,
	}
	decision, err := CheckContentModeration(p.Content)
	if err != nil {
		return 0, err
	}
	if err = mysql.CreateAnswer(a, decision.Status); err != nil {
		return 0, err
	}
	return a.ID, nil
}

// ListAnswers 问题下的回答列表（cursor 分页）
// 问题不存在或已软删除时统一不可见（公开入口不返回已删除内容的数据）
func ListAnswers(questionID int64, order, cursor string, limit int) (*models.PageResult[*models.ApiAnswerDetail], error) {
	question, err := mysql.GetQuestionByID(questionID)
	if err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	// limit 默认 20，最大 50
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if order != models.AnswerOrderHot && order != models.AnswerOrderLatest {
		order = models.AnswerOrderLatest
	}
	// 解析游标
	cp, err := parseAnswerCursor(cursor)
	if err != nil {
		return nil, ErrCursorInvalid
	}
	params := &mysql.AnswerPageParams{
		QuestionID:       questionID,
		Order:            order,
		Limit:            limit,
		AcceptedAnswerID: question.AcceptedAnswerID, // 采纳回答首页置顶、翻页不重复
	}
	if cp != nil {
		params.CursorTime = cp.Time
		params.CursorID = cp.ID
		params.CursorVote = cp.Vote
	}
	answers, err := mysql.ListAnswersByQuestion(params)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(answers) > limit {
		hasMore = true
		answers = answers[:limit]
	}
	items := make([]*models.ApiAnswerDetail, 0, len(answers))
	var last *models.Answer
	for _, a := range answers {
		last = a
		user, err := mysql.GetUserByID(a.AuthorID)
		if err != nil {
			return nil, err
		}
		a.Accepted = question.AcceptedAnswerID == a.ID // 派生标记：采纳回答前端高亮
		items = append(items, &models.ApiAnswerDetail{
			Answer:     a,
			AuthorName: user.Username,
		})
	}
	nextCursor := ""
	if hasMore && last != nil {
		nextCursor = encodeAnswerCursor(order, &answerCursor{
			Time: last.CreateTime,
			ID:   last.ID,
			Vote: last.VoteCount,
		})
	}
	return &models.PageResult[*models.ApiAnswerDetail]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// ---------- 回答游标编解码（opaque，前端不解析） ----------

type answerCursor struct {
	Order string    `json:"o"`
	Time  time.Time `json:"t"`
	ID    int64     `json:"i"`
	Vote  int32     `json:"v"`
}

var ErrCursorInvalid = errors.New("游标不合法")

func encodeAnswerCursor(order string, c *answerCursor) string {
	c.Order = order
	raw := marshalJSONMust(c)
	return base64Encode(raw)
}

func parseAnswerCursor(cursor string) (*answerCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64Decode(cursor)
	if err != nil {
		return nil, ErrCursorInvalid
	}
	var c answerCursor
	if err := unmarshalJSONMust(raw, &c); err != nil {
		return nil, ErrCursorInvalid
	}
	return &c, nil
}
