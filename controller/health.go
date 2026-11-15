package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
)

// HealthzHandler 存活探针：进程在即返回 200，不检查依赖
func HealthzHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ReadyzHandler 就绪探针：检查 MySQL、Redis 与迁移版本，任一失败返回 503
// 用于把未就绪实例移出流量（LB/K8s readiness）
func ReadyzHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	if err := mysql.PingContext(ctx); err != nil {
		zap.L().Error("readyz: mysql ping failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "component": "mysql"})
		return
	}
	if err := redis.PingContext(ctx); err != nil {
		zap.L().Error("readyz: redis ping failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "component": "redis"})
		return
	}
	// 迁移版本校验：确保实例使用的 schema 已至少执行到基线迁移
	latest, err := mysql.LatestMigrationVersion(ctx)
	if err != nil || latest < 0 {
		zap.L().Error("readyz: migration check failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "component": "migrations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "migration_version": latest, "auto_migrate": viper.GetBool("mysql.auto_migrate")})
}
