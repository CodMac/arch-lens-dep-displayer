// Package repository 提供数据访问读模型：面向当前激活源的业务查询。
// 组合数据层 db 的查询能力，供 api service 使用；不关心源/维度切换（连接由外部提供）。
package repository

import (
	"database/sql"
)

// Repository 数据访问读模型。
// 持 func() *sql.DB 而非固定连接：随激活源切换动态取当前连接，无需重建对象。
type Repository struct {
	db func() *sql.DB
}

// New 创建读模型。db 提供当前激活源的连接（由 registry 惰性打开）。
func New(db func() *sql.DB) *Repository {
	return &Repository{db: db}
}
