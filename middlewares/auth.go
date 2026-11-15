package middlewares

import (
	"bluebell/controller"
	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/pkg/jwt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"strings"
)

// jwt 中间件
// 校验：签名与有效期（ParseToken）+ token_version（密码重置后旧 token 立即失效）
func JWTAuthMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 客户端携带 Token 有三种方式 1.放在请求头 2.放在请求体 3.放在URI
		// 这里假设 Token 放在 Header 的 Authorization 中，并使用 Bearer 开头
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			controller.ResponseError(c, controller.CodeNeedLogin)
			c.Abort()
			return
		}

		// 按空格分割，取 Bearer 后面的 token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}
		tokenString := parts[1]

		// 解析 Token（验签 + 过期检查）
		mc, err := jwt.ParseToken(tokenString)
		if err != nil {
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}

		// token_version 校验：Redis 缓存 5 分钟，未命中回查 MySQL
		// 密码重置会 +1 并清缓存，使该用户所有旧 token 立即失效
		version, err := redis.GetTokenVersionCached(mc.UserId)
		if err != nil {
			zap.L().Error("get token version cache failed", zap.Int64("uid", mc.UserId), zap.Error(err))
			controller.ResponseError(c, controller.CodeServerBusy)
			c.Abort()
			return
		}
		if version == -1 {
			version, err = mysql.GetUserTokenVersion(mc.UserId)
			if err != nil {
				zap.L().Error("get token version failed", zap.Int64("uid", mc.UserId), zap.Error(err))
				controller.ResponseError(c, controller.CodeInvalidAuth)
				c.Abort()
				return
			}
			redis.SetTokenVersionCache(mc.UserId, version)
		}
		if version != mc.TokenVersion {
			// 旧会话（密码重置前签发）一律拒绝
			controller.ResponseError(c, controller.CodeInvalidAuth)
			c.Abort()
			return
		}

		// 将解析后的用户信息存储到上下文中
		c.Set("userID", mc.UserId)
		c.Set("userName", mc.UserName)

		c.Next()
	}
}
