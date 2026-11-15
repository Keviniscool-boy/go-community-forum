package middlewares

import (
	"bluebell/controller"
	"bluebell/dao/mysql"
	"bluebell/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequireRoleMiddleware 角色权限中间件：从数据库读取当前角色（带进程内缓存，TTL 由调用方保证足够短），
// 权限不依赖 JWT 内容，角色变更即时生效。
// allowedRoles 为空表示仅要求登录。
func RequireRoleMiddleware(allowedRoles ...string) func(c *gin.Context) {
	return func(c *gin.Context) {
		userID, err := getCurrentUserIDFrom(c)
		if err != nil {
			controller.ResponseError(c, controller.CodeNeedLogin)
			c.Abort()
			return
		}
		if len(allowedRoles) == 0 {
			c.Next()
			return
		}
		role, err := getUserRole(userID)
		if err != nil {
			zap.L().Error("get user role failed", zap.Int64("uid", userID), zap.Error(err))
			controller.ResponseError(c, controller.CodeServerBusy)
			c.Abort()
			return
		}
		for _, r := range allowedRoles {
			if r == role {
				c.Set("role", role)
				c.Next()
				return
			}
		}
		controller.ResponseError(c, controller.CodeForbidden)
		c.Abort()
	}
}

// getCurrentUserIDFrom 兼容 JWT 中间件注入的两种上下文取法
func getCurrentUserIDFrom(c *gin.Context) (int64, error) {
	if v, ok := c.Get("userID"); ok {
		if id, ok := v.(int64); ok {
			return id, nil
		}
	}
	return 0, ErrNotLogin
}

// ErrNotLogin 未登录错误
var ErrNotLogin = errNotLogin{}

type errNotLogin struct{}

func (errNotLogin) Error() string { return "用户未登录" }

// getUserRole 查询用户角色（每次查库，角色变更即时生效；规模小可接受，避免权限缓存绕过）
func getUserRole(userID int64) (string, error) {
	u, err := mysql.GetUserByID(userID)
	if err != nil {
		return "", err
	}
	if u.Role == "" {
		return models.RoleUser, nil
	}
	return u.Role, nil
}
