package mysql

import (
	"database/sql"
	"errors"
	"fmt"

	"bluebell/models"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserExist          = errors.New("用户已存在")
	ErrUserNotExist       = errors.New("用户不存在")
	ErrAccountBanned      = errors.New("账号已被禁用")
)

// CheckUserExist 根据用户名查询用户是否存在
// 参数:
//   - username: 要查询的用户名
//
// 返回:
//   - bool: 用户是否存在 (true 表示存在)
//   - error: 查询过程中产生的错误（无错误时为 nil）
func CheckUserExist(username string) (bool, error) {
	sqlStr := "SELECT COUNT(user_id) FROM user WHERE username = ?"
	var count int
	if err := db.Get(&count, sqlStr, username); err != nil {
		return false, err
	}
	return count > 0, nil
}

// InsertUser 向数据库中插入一条用户记录（旧用户名注册接口使用）
// 该函数会对传入的 `user.Password` 进行 bcrypt 哈希后再存储到数据库；
// 旧接口没有邮箱，写入 user_id 派生的占位邮箱（placeholder.invalid 为 RFC 2606 保留 TLD），
// email_verified 保持默认 0，占位邮箱不能作为登录邮箱
func InsertUser(user *models.User) error {
	sqlStr := "INSERT INTO user (user_id, username, email, password) VALUES (?, ?, ?, ?)"
	// 对密码进行哈希再存储
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(sqlStr, user.UserID, user.Username,
		fmt.Sprintf("legacy_%d@placeholder.invalid", user.UserID), string(hashedPassword))
	if err != nil {
		// 并发注册同名用户时可能撞唯一索引 idx_username（MySQL 错误码 1062），映射为业务错误
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrUserExist
		}
		return err
	}
	return nil
}

// Login 根据用户名查询用户并校验密码
// 说明:
//   - 调用时传入的 `user.Password` 应为登录时提交的明文密码，`user.Username` 为要登录的用户名
//   - 本函数会从数据库读取该用户（并填充 `user.UserID`、`user.Username`、`user.Password`，其中 `user.Password` 为数据库中的哈希值）
//   - 然后用 bcrypt 比较数据库中的哈希与传入的明文密码，校验通过返回 nil，失败返回错误
//
// 参数:
//   - user: 传入包含 `Username` 和明文 `Password` 的用户对象；函数会在成功时填充更多字段
//
// 返回:
//   - error: 查询或校验过程中发生的错误（密码错误时也会返回错误）
func Login(user *models.User) (err error) {
	oPassword := user.Password
	sqlStr := "SELECT user_id, username, password, token_version, status FROM user WHERE username = ?"
	if err = db.Get(user, sqlStr, user.Username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return err
	}
	// 禁用账号统一拒绝
	if user.Status == 0 {
		return ErrAccountBanned
	}
	// 判断密码是否正确（user.Password 为数据库中读取的哈希）
	if err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oPassword)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		}
		return err
	}
	return nil

}

// GetUserByID 根据用户id查询用户信息
func GetUserByID(id int64) (user *models.User, err error) {
	user = new(models.User)
	sqlStr := "SELECT user_id, username FROM user WHERE user_id = ?"
	if err = db.Get(user, sqlStr, id); err != nil {
		return nil, err
	}
	return user, nil
}

// CheckEmailExist 查询邮箱是否已被注册
func CheckEmailExist(email string) (bool, error) {
	sqlStr := "SELECT COUNT(user_id) FROM user WHERE email = ?"
	var count int
	if err := db.Get(&count, sqlStr, email); err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckUsernameExist 查询用户名是否已被占用
func CheckUsernameExist(username string) (bool, error) {
	return CheckUserExist(username)
}

// CreateEmailUser 创建邮箱注册用户（密码 bcrypt 哈希后存储，邮箱视为已验证）
func CreateEmailUser(user *models.User) error {
	sqlStr := "INSERT INTO user (user_id, username, email, email_verified, password) VALUES (?, ?, ?, 1, ?)"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(sqlStr, user.UserID, user.Username, user.Email, string(hashedPassword))
	if err != nil {
		// 撞唯一索引（邮箱或用户名已存在），映射为业务错误
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrUserExist
		}
	}
	return err
}

// GetUserByEmail 根据邮箱查询用户（不存在返回 sql.ErrNoRows）
func GetUserByEmail(email string) (user *models.User, err error) {
	user = new(models.User)
	sqlStr := "SELECT user_id, username, email, email_verified, password, avatar_url, bio, status, token_version FROM user WHERE email = ?"
	if err = db.Get(user, sqlStr, email); err != nil {
		return nil, err
	}
	return user, nil
}

// LoginByEmail 邮箱密码登录校验：按邮箱取用户并比对 bcrypt 哈希
func LoginByEmail(user *models.User) error {
	oPassword := user.Password
	u, err := GetUserByEmail(user.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(oPassword)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		}
		return err
	}
	// 把查到的用户信息回填，供 logic 层生成 token 使用
	*user = *u
	return nil
}

// UpdateLastLogin 更新最后登录时间
func UpdateLastLogin(userID int64) error {
	sqlStr := "UPDATE user SET last_login_at = NOW() WHERE user_id = ?"
	_, err := db.Exec(sqlStr, userID)
	return err
}

// GetUserDetailByID 查询用户完整信息（供 /auth/me 使用）
func GetUserDetailByID(id int64) (user *models.User, err error) {
	user = new(models.User)
	sqlStr := "SELECT user_id, username, email, email_verified, avatar_url, bio, status FROM user WHERE user_id = ?"
	if err = db.Get(user, sqlStr, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotExist
		}
		return nil, err
	}
	return user, nil
}

// GetUserTokenVersion 查询用户的 JWT 版本号（中间件校验用）
func GetUserTokenVersion(userID int64) (int, error) {
	var v int
	err := db.Get(&v, "SELECT token_version FROM user WHERE user_id = ?", userID)
	return v, err
}

// UpdatePasswordByEmail 重置密码并使所有旧会话失效（token_version +1）
// 返回是否实际更新（邮箱不存在时 affected=0，调用方做统一响应，不泄露存在性）
func UpdatePasswordByEmail(email, plainPassword string) (bool, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	result, err := db.Exec(
		"UPDATE user SET password = ?, token_version = token_version + 1 WHERE email = ? AND email_verified = 1",
		string(hashed), email)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// CountUserQuestions 用户提问数
func CountUserQuestions(userID int64) (int, error) {
	var n int
	err := db.Get(&n, "SELECT COUNT(question_id) FROM question WHERE author_id = ? AND status = 1", userID)
	return n, err
}

// CountUserAnswers 用户回答数
func CountUserAnswers(userID int64) (int, error) {
	var n int
	err := db.Get(&n, "SELECT COUNT(answer_id) FROM answer WHERE author_id = ? AND status = 1", userID)
	return n, err
}

// ListUserQuestions 用户的提问列表（最新优先）
func ListUserQuestions(userID int64, limit int) (questions []*models.Question, err error) {
	questions = make([]*models.Question, 0, limit)
	err = db.Select(&questions, `SELECT question_id, title, description, author_id, answer_count,
		comment_count, follower_count, view_count, status, create_time
		FROM question WHERE author_id = ? AND status = 1 ORDER BY create_time DESC LIMIT ?`, userID, limit)
	return
}

// ListUserAnswers 用户的回答列表（带所属问题标题）
func ListUserAnswers(userID int64, limit int) (answers []*models.ApiAnswerDetail, err error) {
	answers = make([]*models.ApiAnswerDetail, 0, limit)
	err = db.Select(&answers, `SELECT a.answer_id, a.question_id, a.author_id, a.content, a.vote_count,
		a.comment_count, a.collect_count, a.status, a.create_time, qu.title AS question_title
		FROM answer a JOIN question qu ON qu.question_id = a.question_id
		WHERE a.author_id = ? AND a.status = 1 ORDER BY a.create_time DESC LIMIT ?`, userID, limit)
	return
}
