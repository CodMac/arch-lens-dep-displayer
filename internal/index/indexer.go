// Package index 提供索引构建器：把主表数据加工成可查询的层级/聚合表。
package index

import (
	"database/sql"

	"github.com/CodMac/arch-lens-dep-displayer/internal/dimension"
)

// Indexer 索引构建器：主表（elements/relations）→ 层级表 + 预计算聚合表。
// 生命周期短：每次分析按源执行一遍即弃，由 pipeline 使用。
type Indexer struct {
	db  *sql.DB
	dim *dimension.Dimension
}

// New 创建索引构建器。
func New(db *sql.DB, dim *dimension.Dimension) *Indexer {
	return &Indexer{db: db, dim: dim}
}

// Build 执行完整索引构建：层级树 + 预计算。
func (ix *Indexer) Build() error {
	if err := ix.buildHierarchy(); err != nil {
		return err
	}
	return ix.precompute()
}
