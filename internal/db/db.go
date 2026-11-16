// Package db 是 DuckDB 数据访问层：只负责连接生命周期。
// 表约束见 schema.go；全部 SQL（含 JOIN/聚合）见 repository 子包。
package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/CodMac/arch-lens-dep-displayer/internal/duckdbthin"
)

// Open 打开（或创建）DuckDB 数据库文件。
// dbPath 为空时使用内存库（适合测试）。
func Open(dbPath string) (*sql.DB, error) {
	dsn := ":memory:"
	if dbPath != "" {
		dsn = dbPath
	}
	c, err := sql.Open("duckdbthin", dsn)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %q: %w", dsn, err)
	}
	// DuckDB 内存库每个连接是独立实例；连接池多连接会看到不同库。
	// 强制单连接保证 schema 与数据一致。
	c.SetMaxOpenConns(1)
	c.SetMaxIdleConns(1)
	if err := c.Ping(); err != nil {
		return nil, fmt.Errorf("ping duckdb: %w", err)
	}
	return c, nil
}

// Close 关闭数据库连接。
func Close(c *sql.DB) error {
	if c == nil {
		return nil
	}
	return c.Close()
}

// Recreate 删除旧库文件并新建空库（含表约束）。
// 命令行工具定位：索引全量可重建，不做跨版本兼容，结构变更直接删库。
func Recreate(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("recreate: 需要文件路径")
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove duckdb %q: %w", dbPath, err)
	}
	c, err := Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := Init(c); err != nil {
		Close(c)
		return nil, err
	}
	return c, nil
}

// Ensure 打开库并校验 schema 版本；版本不符（或旧库无版本表）则删库重建。
func Ensure(dbPath string) (*sql.DB, error) {
	c, err := Open(dbPath)
	if err != nil {
		return nil, err
	}
	if ok := schemaCurrent(c); ok {
		return c, nil
	}
	Close(c)
	return Recreate(dbPath)
}
