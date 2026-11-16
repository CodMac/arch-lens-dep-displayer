// Package preprocess 提供预处理构建器：把主表数据加工成可查询的层级/聚合表。
package preprocess

import "github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"

// Preprocessor 预处理构建器：主表（elements/relations）→ 层级表 + 预计算聚合表。
// 生命周期短：每次分析按源执行一遍即弃，由 pipeline 使用。
type Preprocessor struct {
	repo *repository.Repository
	dim  *Dimension
}

// New 创建预处理构建器。
func New(repo *repository.Repository, dim *Dimension) *Preprocessor {
	return &Preprocessor{repo: repo, dim: dim}
}

// Logf 进度日志钩子（由调用方注入，默认丢弃）。
var Logf = func(format string, args ...interface{}) {}

// Build 执行完整预处理：层级树 + 预计算。
func (ix *Preprocessor) Build() error {
	Logf("hierarchy: start")
	if err := ix.buildHierarchy(); err != nil {
		return err
	}
	Logf("hierarchy: done")
	return ix.precompute()
}
