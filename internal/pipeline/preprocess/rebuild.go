package preprocess

import (
	"fmt"
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// LoadDimension 从 dimensions / dimension_levels 表重建维度配置（供重算使用）。
func LoadDimension(repo *repository.Repository, dimID int) (*Dimension, error) {
	d := &Dimension{ID: dimID}
	language, sep, err := repo.DimensionHeader(dimID)
	if err != nil {
		return nil, fmt.Errorf("load dimension %d: %w", dimID, err)
	}
	d.Language, d.QNSeparator = language, sep
	levels, err := repo.DimensionLevels(dimID)
	if err != nil {
		return nil, err
	}
	for _, lv := range levels {
		d.Hierarchy = append(d.Hierarchy, Level{Level: lv.Level, Role: lv.Role, Kinds: lv.Kinds})
	}
	// 关系类型白名单来自维度配置；旧库该列为空时从 relations 反推兜底。
	relTypes, structTypes, err := repo.DimensionRelationTypes(dimID)
	if err != nil {
		return nil, err
	}
	d.RelationTypes = splitCSV(relTypes)
	d.StructuralRelationTypes = splitCSV(structTypes)
	if len(d.StructuralRelationTypes) == 0 {
		d.StructuralRelationTypes = []string{"CONTAIN", "IMPORT", "IMPL_LINK"}
	}
	if len(d.RelationTypes) == 0 {
		types, err := repo.RelationTypes(dimID)
		if err != nil {
			return nil, err
		}
		d.RelationTypes = types
	}
	return d, nil
}

// splitCSV 解析逗号分隔串（忽略空项）。
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// clearDerived 清空某维度的全部预计算表（preprocess 全量重跑时幂等）。
func (ix *Preprocessor) clearDerived() error {
	dimID := ix.dim.ID
	for _, clear := range []func(int) error{
		ix.repo.ClearTypeEdge,
		ix.repo.ClearCallSite,
		ix.repo.ClearTypeFan,
		ix.repo.ClearViolationHit,
		ix.repo.ClearCycles,
		ix.repo.ClearPkgRollup,
		ix.repo.ClearMetrics,
	} {
		if err := clear(dimID); err != nil {
			return fmt.Errorf("clear precompute: %w", err)
		}
	}
	return nil
}

// clearViolationDerived 清空违规相关派生表（PUT 改口径后重算，type_edge/type_fan 不动）。
func (ix *Preprocessor) clearViolationDerived() error {
	dimID := ix.dim.ID
	for _, clear := range []func(int) error{
		ix.repo.ClearViolationHit,
		ix.repo.ClearCycles,
		ix.repo.ClearPkgRollup,
	} {
		if err := clear(dimID); err != nil {
			return fmt.Errorf("clear derived: %w", err)
		}
	}
	return nil
}

// RebuildDerived 按当前 violation_config 重建派生表（violation_hit / cycle_* / pkg_rollup）。
// 由 PUT /violations/config 触发；type_edge / type_fan 不动（毫秒级）。
func RebuildDerived(repo *repository.Repository, dimID int) error {
	dim, err := LoadDimension(repo, dimID)
	if err != nil {
		return err
	}
	ix := New(repo, dim)
	if err := ix.clearViolationDerived(); err != nil {
		return err
	}
	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"violation_hit", ix.buildViolationHit},
		{"cycle", ix.buildCycles},
		{"pkg_rollup", ix.buildPkgRollup},
	} {
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}
