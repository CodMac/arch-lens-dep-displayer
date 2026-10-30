// Package db 是 DuckDB 数据访问层：连接生命周期 + 表定义 + 主表操作。
// 职责边界：只做"数据怎么存/取"，不含任何业务语义。
package db

import (
	"database/sql"
	"fmt"

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

// Init 执行建表语句 + 兼容旧库迁移（新列补默认值）。
func Init(c *sql.DB) error {
	if _, err := c.Exec(Schema); err != nil {
		return fmt.Errorf("store init schema: %w", err)
	}
	// 旧库（D0 及更早）的 dimensions 表无 imported_at 列：补列，回填导入时间。
	if _, err := c.Exec(`ALTER TABLE dimensions ADD COLUMN IF NOT EXISTS imported_at TIMESTAMP`); err != nil {
		return fmt.Errorf("store migrate dimensions: %w", err)
	}
	if _, err := c.Exec(`UPDATE dimensions SET imported_at = current_timestamp WHERE imported_at IS NULL`); err != nil {
		return fmt.Errorf("store backfill imported_at: %w", err)
	}
	return nil
}
