// Package repository 承载全部 SQL：按「主表」分文件组织增删查改。
// 允许 JOIN / 聚合 / 内存组装，但方法归属以主表为准（FROM 的第一张表 / 被写入的表）。
// 连接由外部提供：读路径随激活源切换（New），写路径用固定连接（Of）。
package repository

import (
	"database/sql"
)

// Repository 数据访问层。
// 持 func() *sql.DB 而非固定连接：随激活源切换动态取当前连接，无需重建对象。
type Repository struct {
	db func() *sql.DB
}

// New 创建读路径仓库。db 提供当前激活源的连接（由 registry 惰性打开）。
func New(db func() *sql.DB) *Repository {
	return &Repository{db: db}
}

// Of 创建固定连接的仓库（写路径：pipeline）。
func Of(c *sql.DB) *Repository {
	return &Repository{db: func() *sql.DB { return c }}
}
