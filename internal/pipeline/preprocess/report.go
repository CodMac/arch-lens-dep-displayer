package preprocess

import (
	"fmt"
	"sort"

	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline/proclog"
)

// sortedKeys 返回 map 的 key 升序切片（日志输出稳定）。
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// reportAnomalies 检查派生层的一致性并把明细/汇总写入过程日志（只读，不修补）。
// 检查项：游离元素、游离类型、悬空归属、层级孤儿、默认包元素、空表。
func (ix *Preprocessor) reportAnomalies(log *proclog.Logger) {
	if log == nil {
		return
	}
	dimID := ix.dim.ID
	const detailLimit = 200

	// 1. 游离元素：elements 中有、element_node 中无（未纳入层级体系）。
	if total, list, err := ix.repo.UnattachedElements(dimID, detailLimit); err != nil {
		log.Warn("preprocess:unattached-element", "", "err", err.Error())
	} else {
		for _, e := range list {
			log.Warn("preprocess:unattached-element", "", "qn", e.QN, "kind", e.Kind)
		}
		log.Summary("preprocess:unattached-element", "total", total, "emitted", len(list))
	}

	// 2. 游离类型：type_fan 中 fan_in=0 且 fan_out=0（无跨类型边）。
	if total, list, err := ix.repo.IsolatedTypes(dimID, detailLimit); err != nil {
		log.Warn("preprocess:isolated-type", "", "err", err.Error())
	} else {
		for _, e := range list {
			log.Warn("preprocess:isolated-type", "", "qn", e.QN, "kind", e.Kind)
		}
		log.Summary("preprocess:isolated-type", "total", total, "emitted", len(list))
	}

	// 3. 悬空归属：namespace_qn / enclosing_qn / type_qn 指向不存在的元素。
	if total, list, err := ix.repo.DanglingOwnership(detailLimit); err != nil {
		log.Warn("preprocess:dangling-ownership", "", "err", err.Error())
	} else {
		for _, o := range list {
			log.Warn("preprocess:dangling-ownership", "", "qn", o.QN, "kind", o.Kind,
				"field", o.Field, "value", o.Value)
		}
		log.Summary("preprocess:dangling-ownership", "total", total, "emitted", len(list))
	}

	// 4. 层级孤儿：parent_qn 指向不存在的层级节点。
	if total, list, err := ix.repo.OrphanHierarchyNodes(dimID, detailLimit); err != nil {
		log.Warn("preprocess:orphan-node", "", "err", err.Error())
	} else {
		for _, n := range list {
			log.Warn("preprocess:orphan-node", "", "qn", n.QN, "kind", n.Kind,
				"level", n.Level, "parent_qn", n.Parent)
		}
		log.Summary("preprocess:orphan-node", "total", total, "emitted", len(list))
	}

	// 5. 默认包元素：namespace_qn 为空（已知 analyzer 缺陷，只在查询期兜底）。
	nsLevel := ix.dim.LevelOfRole(RoleNamespace)
	nsKinds := ix.dim.KindsOfLevel(nsLevel)
	kindList := make([]string, 0, len(nsKinds))
	for k := range nsKinds {
		kindList = append(kindList, k)
	}
	if total, list, err := ix.repo.ElementsMissingNamespace(kindList, detailLimit); err != nil {
		log.Warn("preprocess:missing-namespace", "", "err", err.Error())
	} else {
		for _, e := range list {
			log.Warn("preprocess:missing-namespace", "", "qn", e.QN, "kind", e.Kind)
		}
		log.Summary("preprocess:missing-namespace", "total", total, "emitted", len(list))
	}

	// 6. 空表检查：派生表行数为 0 视为异常（hierarchy_node / type_edge / type_fan / metrics）。
	if counts, err := ix.repo.TableCounts(dimID); err != nil {
		log.Warn("preprocess:table-counts", "", "err", err.Error())
	} else {
		required := []string{"hierarchy_node", "element_node", "type_edge", "type_fan", "metrics"}
		for _, t := range required {
			if counts[t] == 0 {
				log.Error("preprocess:empty-table", "", "table", t)
			}
		}
		keys := []string{"hierarchy_node", "element_node", "type_edge", "call_site",
			"type_fan", "violation_hit", "cycle_group", "pkg_rollup", "metrics"}
		for _, t := range keys {
			log.Info("preprocess:table-counts", "", "table", t, "rows", counts[t])
		}
	}

	// 7. 未知关系类型：relations.type 不在维度白名单（依赖 ∪ 结构）内。
	allowed := map[string]bool{}
	for _, t := range ix.dim.RelationTypeList() {
		allowed[t] = true
	}
	for _, t := range ix.dim.StructuralTypes() {
		allowed[t] = true
	}
	if hist, err := ix.repo.RelationTypeHistogram(); err != nil {
		log.Warn("preprocess:unknown-relation-type", "", "err", err.Error())
	} else {
		unknown := 0
		for _, t := range sortedKeys(hist) {
			if !allowed[t] {
				unknown++
				log.Warn("preprocess:unknown-relation-type", "", "type", t, "count", hist[t])
			}
		}
		log.Summary("preprocess:unknown-relation-type", "total", unknown)
	}

	// 8. 环组统计。
	if groups, err := ix.repo.CycleGroupCount(dimID); err == nil {
		log.Summary("preprocess:cycles", "groups", groups)
	}

	log.Summary("preprocess", "dimension", fmt.Sprintf("%s(id=%d)", ix.dim.Language, dimID))
}
