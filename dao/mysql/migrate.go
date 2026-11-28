package mysql

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"bluebell/models/migrations"

	"go.uber.org/zap"
)

// 版本化迁移：启动时按文件名顺序执行 models/migrations/ 下未应用的 .sql，
// 版本记录在 schema_migrations 表；GET_LOCK 防止多实例并发执行。
// 空库（全量建表）、Compose 基线库、旧生产库升级走同一条链。

const (
	migrationLockKey  = "bluebell_migration"
	migrationLockWait = 5 * time.Second
	migrationLockTTL  = 120 // 秒，防止持锁实例崩溃后锁泄漏
)

type migrationFile struct {
	Version int
	Name    string
	SQL     string
}

// RunMigrations 执行未应用的迁移（是否调用由 main 依据 mysql.auto_migrate 配置决定）
func RunMigrations() error {
	if err := ensureMigrationTable(); err != nil {
		return err
	}

	// 获取迁移锁，未拿到说明另一实例正在执行，直接跳过（不视为错误）
	var locked int
	if err := db.Get(&locked, "SELECT GET_LOCK(?, ?)", migrationLockKey, int(migrationLockWait.Seconds())); err != nil {
		return fmt.Errorf("acquire migration lock failed: %w", err)
	}
	if locked != 1 {
		zap.L().Warn("another instance holds migration lock, skip")
		return nil
	}
	defer func() {
		if _, err := db.Exec("SELECT RELEASE_LOCK(?)", migrationLockKey); err != nil {
			zap.L().Error("release migration lock failed", zap.Error(err))
		}
	}()

	files, err := listMigrationFiles()
	if err != nil {
		return err
	}

	for _, m := range files {
		applied, err := isMigrationApplied(m.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(m); err != nil {
			return fmt.Errorf("apply migration %03d_%s failed: %w", m.Version, m.Name, err)
		}
		zap.L().Info("migration applied", zap.Int("version", m.Version), zap.String("name", m.Name))
	}
	return nil
}

// ensureMigrationTable 创建迁移版本表（幂等）
func ensureMigrationTable() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT NOT NULL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create schema_migrations failed: %w", err)
	}
	return nil
}

// listMigrationFiles 读取嵌入的 SQL 文件，按文件名（即版本号）排序
func listMigrationFiles() ([]*migrationFile, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, err
	}
	files := make([]*migrationFile, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") || e.IsDir() {
			continue
		}
		version, err := strconv.Atoi(name[:3])
		if err != nil {
			return nil, fmt.Errorf("migration file %s must start with 3-digit version", name)
		}
		content, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return nil, err
		}
		files = append(files, &migrationFile{
			Version: version,
			Name:    strings.TrimSuffix(name, ".sql"),
			SQL:     string(content),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

func isMigrationApplied(version int) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(version) FROM schema_migrations WHERE version = ?", version)
	return count > 0, err
}

// applyMigration 在单个事务内执行一个迁移的全部语句并记录版本
// 注意：MySQL 的 DDL 会隐式提交，事务无法保证 DDL 原子回滚；
// 失败时迁移停在半途（dirty），需要人工处理后手动删除/修正 schema_migrations 记录再重试
func applyMigration(m *migrationFile) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, stmt := range splitSQLStatements(m.SQL) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("statement failed: %s: %w", firstLine(stmt), err)
		}
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.Version, m.Name); err != nil {
		return err
	}
	return tx.Commit()
}

// splitSQLStatements 去掉 -- 注释行后按分号拆分语句
// 迁移文件约定：字符串字面量内不包含分号
func splitSQLStatements(content string) []string {
	var sb strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	statements := make([]string, 0, 8)
	for _, stmt := range strings.Split(sb.String(), ";") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
