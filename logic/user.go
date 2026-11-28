package logic

import (
	"encoding/json"
	"strconv"
	"errors"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/jwt"
	"bluebell/pkg/snowflake"
)

// 业务错误定义
var (
	ErrUserExist          = errors.New("用户已存在")
	ErrPasswordMismatch   = errors.New("两次密码不一致")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
)

// SignUp 用户注册业务逻辑
func SignUp(p *models.ParamSignUp) error {
	// 1. 判断两次密码是否一致
	if p.Password != p.RePassword {
		return ErrPasswordMismatch
	}

	// 2. 查询数据库，查看用户是否存在
	exist, err := mysql.CheckUserExist(p.Username)
	if err != nil {
		return err
	}
	if exist {
		return ErrUserExist
	}

	// 3. 生成 UID
	userID := snowflake.GenerateID()

	// 4. 保存进数据库（密码加密在 mysql 层处理）
	//构造一个 User 实例
	user := &models.User{
		UserID:   userID,
		Username: p.Username,
		Password: p.Password,
	}
	if err := mysql.InsertUser(user); err != nil {
		// 并发注册时撞唯一索引，InsertUser 返回 mysql.ErrUserExist，转成业务错误
		if errors.Is(err, mysql.ErrUserExist) {
			return ErrUserExist
		}
		return err
	}
	return nil
}

func Login(p *models.ParamLogin) (token string, err error) {
	//构造一个 User 实例
	user := &models.User{
		Username: p.Username,
		Password: p.Password,
	}
	// 调用 mysql 层的 Login 函数进行登录验证
	if err := mysql.Login(user); err != nil {
		if errors.Is(err, mysql.ErrInvalidCredentials) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	//生成jwt（携带 token_version，密码重置后旧 token 失效）
	return jwt.GenToken(user.UserID, user.Username, user.TokenVersion)

}

// ErrInvalidNotifySetting 通知偏好键不合法
var ErrInvalidNotifySetting = errors.New("通知偏好键不合法")

// GetNotifySettings 读通知偏好（缺省键返回 true）
func GetNotifySettings(userID int64) (map[string]bool, error) {
	raw, err := mysql.GetNotifySettingsRaw(userID)
	if err != nil {
		return nil, err
	}
	keys := []string{"like", "comment", "reply", "collect", "dm"}
	out := make(map[string]bool, len(keys))
	var parsed map[string]*bool
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	for _, k := range keys {
		v, ok := parsed[k]
		out[k] = !ok || v == nil || *v
	}
	return out, nil
}

// SetNotifySettings 写通知偏好（全量覆盖；键校验）
func SetNotifySettings(userID int64, pref map[string]bool) error {
	valid := map[string]bool{"like": true, "comment": true, "reply": true, "collect": true, "dm": true}
	for k := range pref {
		if !valid[k] {
			return ErrInvalidNotifySetting
		}
	}
	raw, err := json.Marshal(pref)
	if err != nil {
		return err
	}
	return mysql.SetNotifySettings(userID, raw)
}

// SetQuestionRedirect 治理重定向（PRD F8；仅 admin，controller 层走 RequireRole）
func SetQuestionRedirect(operatorID, questionID, targetID int64, note string) error {
	if err := mysql.ValidateRedirectTarget(questionID, targetID); err != nil {
		return err
	}
	if err := mysql.SetQuestionRedirect(questionID, targetID, operatorID); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]interface{}{
		"target_id": strconv.FormatInt(targetID, 10),
		"note":      note,
	})
	_ = mysql.CreateAuditLog(&models.AuditLog{
		AuditID:  snowflake.GenerateID(),
		ActorID:  operatorID,
		Action:   "question_redirect",
		TargetType: "question",
		TargetID: strconv.FormatInt(questionID, 10),
		RequestID: "",
		Metadata: string(meta),
	})
	return nil
}

// ClearQuestionRedirect 清除重定向（可逆）
func ClearQuestionRedirect(operatorID, questionID int64) error {
	if err := mysql.SetQuestionRedirect(questionID, 0, operatorID); err != nil {
		return err
	}
	_ = mysql.CreateAuditLog(&models.AuditLog{
		AuditID:  snowflake.GenerateID(),
		ActorID:  operatorID,
		Action:   "question_redirect_clear",
		TargetType: "question",
		TargetID: strconv.FormatInt(questionID, 10),
		Metadata: "{}",
	})
	return nil
}
