package middlewares

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const HeaderRequestID = "X-Request-ID"

// genRequestID 生成 16 字节随机十六进制请求 ID
func genRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "req-fallback"
	}
	return hex.EncodeToString(b)
}

// RequestIDMiddleware 生成或透传 X-Request-ID：
//   - 客户端带了合法 ID（≤64 字符）则透传复用，否则生成新 ID
//   - 写入上下文（供审计/事件/日志使用）并回写响应头
// 禁止把 token 当 request ID
func RequestIDMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if len(id) == 0 || len(id) > 64 {
			id = genRequestID()
		}
		c.Set("request_id", id)
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}

// GetRequestID 从上下文取请求 ID（未经过中间件时返回空串）
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
