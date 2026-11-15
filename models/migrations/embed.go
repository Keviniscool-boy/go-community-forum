// Package migrations 内嵌全部 SQL 迁移文件
// 迁移链是唯一的建表/升级路径：空库、Compose 初始化库（仅基线）、旧生产库
// 都按文件名顺序执行同一条链，由 dao/mysql.RunMigrations 记录版本
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
