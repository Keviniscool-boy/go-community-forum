package middlewares

import (
	"bluebell/controller"
	"bluebell/pkg/jwt"
	"github.com/gin-gonic/gin"
	"strings"
)

// OptionalAuthMiddleware 可选鉴权：用于公开 Feed 与需要"当前用户互动状态"的公开详情。
//   - 无 token：放行，上下文无用户（游客）
//   - 无效 token（签名错/过期/版本不符）：直接返回认证错误，绝不静默当游客处理
//   - 有效 token：注入 userID/userName
func OptionalAuthMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			// 游客
			c.Next()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}
		mc, err := jwt.ParseToken(parts[1])
		if err != nil {
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}
		// token_version 校验与强制鉴权一致（密码重置/全局登出后旧 token 失效）
		version, err := currentTokenVersion(mc.UserId)
		if err != nil {
			controller.ResponseError(c, controller.CodeServerBusy)
			c.Abort()
			return
		}
		if version != mc.TokenVersion {
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}
		c.Set("userID", mc.UserId)
		c.Set("userName", mc.UserName)
		c.Next()
	}
}
