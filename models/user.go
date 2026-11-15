package models

import "time"

// User 数据库用户模型
// Password 存 bcrypt 哈希；JSON 输出中不包含密码
type User struct {
	UserID       int64     `json:"user_id,string" db:"user_id"`
	Username     string    `json:"username" db:"username"`
	Email        string    `json:"email" db:"email"`
	EmailVerified bool     `json:"email_verified" db:"email_verified"`
	Password     string    `json:"-" db:"password"`
	AvatarURL    string    `json:"avatar_url" db:"avatar_url"`
	Bio          string    `json:"bio" db:"bio"`
	Status       int32     `json:"status" db:"status"`
	LastLoginAt  time.Time `json:"last_login_at" db:"last_login_at"`
}
