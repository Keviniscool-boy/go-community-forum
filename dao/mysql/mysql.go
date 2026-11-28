package mysql

import (
	"database/sql"
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var db *sqlx.DB

// Init 初始化 MySQL 连接
func Init() error {
	// 读取配置
	host := viper.GetString("mysql.host")
	port := viper.GetInt("mysql.port")
	user := viper.GetString("mysql.user")
	password := viper.GetString("mysql.password")
	dbname := viper.GetString("mysql.dbname")

	// 构建 DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
		user, password, host, port, dbname)

	// 连接数据库
	var err error
	db, err = sqlx.Connect("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql connect failed: %w", err)
	}

	// 连接池配置
	db.SetMaxOpenConns(100)                 // 最大打开连接数
	db.SetMaxIdleConns(10)                  // 最大空闲连接数
	db.SetConnMaxLifetime(30 * time.Minute) // 连接最大存活时间

	// 测试连接
	if err = db.Ping(); err != nil {
		return fmt.Errorf("mysql ping failed: %w", err)
	}

	zap.L().Info("MySQL connected",
		zap.String("host", host),
		zap.Int("port", port),
		zap.String("dbname", dbname),
	)

	return nil
}

// Close 关闭数据库连接
func Close() {
	if db != nil {
		if err := db.Close(); err != nil {
			zap.L().Error("mysql close failed", zap.Error(err))
		}
	}
}

// GetDB 获取数据库实例
func GetDB() *sqlx.DB {
	return db
}

// setGlobalDB 测试用：替换全局数据库连接（migrate_test.go 使用）
// SetGlobalDB / GetGlobalDB 供跨包集成测试与只读批查使用（写路径一律走 DAO 函数）
func SetGlobalDB(d *sqlx.DB) { setGlobalDB(d) }

func GetGlobalDB() *sqlx.DB { return db }

func setGlobalDB(d *sqlx.DB) {
	db = d
}

// PingContext 健康检查：验证连接可用
func PingContext(ctx context.Context) error {
	return db.PingContext(ctx)
}

// LatestMigrationVersion 当前已应用的最大迁移版本（就绪探针用）；无版本记录返回 -1
func LatestMigrationVersion(ctx context.Context) (int, error) {
	var v int
	err := db.GetContext(ctx, &v,
		"SELECT COALESCE(MAX(version), -1) FROM schema_migrations")
	return v, err
}

// TxLike 事务句柄的最小接口（*sqlx.Tx 满足；便于跨 DAO 组合事务）
type TxLike interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// WithTx 在单事务内执行 fn：成功提交，失败回滚并返回错误
func WithTx(fn func(tx TxLike) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	handle := TxLike(tx)
	if err = fn(handle); err != nil {
		return err
	}
	return tx.Commit()
}
