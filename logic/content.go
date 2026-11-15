package logic

import (
	"errors"

	"bluebell/dao/mysql"
	"bluebell/models"
)

// 内容管理业务错误
var ErrForbidden = errors.New("无权操作该内容")

// EditQuestion 作者编辑问题
func EditQuestion(userID int64, questionID int64, p *models.ParamUpdateQuestion) error {
	updated, err := mysql.UpdateQuestion(questionID, userID, p.Title, p.Description)
	if err != nil {
		return err
	}
	if !updated {
		return ErrForbidden // 问题不存在或非作者
	}
	return nil
}

// DeleteQuestion 作者软删除问题
func DeleteQuestion(userID, questionID int64) error {
	deleted, err := mysql.SoftDeleteQuestion(questionID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrForbidden
	}
	return nil
}

// EditAnswer 作者编辑回答
func EditAnswer(userID, answerID int64, content string) error {
	updated, err := mysql.UpdateAnswer(answerID, userID, content)
	if err != nil {
		return err
	}
	if !updated {
		return ErrForbidden
	}
	return nil
}

// DeleteAnswer 作者软删除回答（回答数在 dao 事务内重算）
func DeleteAnswer(userID, answerID int64) error {
	deleted, err := mysql.SoftDeleteAnswer(answerID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrForbidden
	}
	return nil
}

// UpdateProfile 更新个人资料
func UpdateProfile(userID int64, p *models.ParamUpdateProfile) error {
	return mysql.UpdateProfile(userID, p.Bio, p.AvatarURL)
}
