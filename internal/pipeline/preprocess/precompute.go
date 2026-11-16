package preprocess

import (
	"fmt"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// precompute 执行全部预计算（对齐 docs/architecture.md §5.2）。
// 顺序有依赖：type_edge → call_site → type_fan → violation → cycle → pkg_rollup → metrics。
func (ix *Preprocessor) precompute() error {
	if err := ix.clearDerived(); err != nil {
		return err
	}
	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"type_edge", ix.buildTypeEdge},
		{"call_site", ix.buildCallSite},
		{"type_fan", ix.buildTypeFan},
		{"violation_config", ix.ensureViolationConfig},
		{"violation_hit", ix.buildViolationHit},
		{"cycle", ix.buildCycles},
		{"pkg_rollup", ix.buildPkgRollup},
		{"metrics", ix.buildMetrics},
	} {
		Logf("precompute %s: start", step.name)
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		Logf("precompute %s: done", step.name)
	}
	return nil
}

// edgeFilter 返回类型级边/调用点的关系过滤条件（维度配置驱动）。
func (ix *Preprocessor) edgeFilter() repository.TypeEdgeFilter {
	return repository.TypeEdgeFilter{
		RelationTypes:   ix.dim.RelationTypeList(),
		StructuralTypes: ix.dim.StructuralTypes(),
	}
}

// buildTypeEdge 类型级边（P3）：端点归一化到 type_qn，加权 + 去重引用点，标记跨包。
func (ix *Preprocessor) buildTypeEdge() error {
	return ix.repo.BuildTypeEdge(ix.dim.ID, ix.edgeFilter())
}

// buildCallSite 调用点明细（P5）：端点保留元素自身 QN + kind，无 Location 写空 path / line=0。
func (ix *Preprocessor) buildCallSite() error {
	return ix.repo.BuildCallSite(ix.dim.ID, ix.edgeFilter())
}

// buildTypeFan 类型级扇入扇出（P6）：与阈值无关的原始指标，孤立类型落全 0 行。
func (ix *Preprocessor) buildTypeFan() error {
	return ix.repo.BuildTypeFan(ix.dim.ID, ix.dim.LevelOfRole(RoleType))
}

// ensureViolationConfig 首次 preprocess 写入默认口径（已存在则保留用户设置）。
func (ix *Preprocessor) ensureViolationConfig() error {
	return ix.repo.EnsureViolationConfig(ix.dim.ID, ix.dim.RelationTypeList())
}

// buildViolationHit 按当前 violation_config 阈值从 type_fan 命中（P7）。
func (ix *Preprocessor) buildViolationHit() error {
	return ix.repo.BuildViolationHit(ix.dim.ID)
}

// buildCycles 类型级环（P8）：按 cycle_relation_types 过滤 → Tarjan → 仅保留跨包组。
func (ix *Preprocessor) buildCycles() error {
	dimID := ix.dim.ID
	allowed := ix.repo.CycleRelationTypes(dimID, ix.dim.RelationTypeList())

	edges, err := ix.repo.TypeEdgesWithTypes(dimID)
	if err != nil {
		return err
	}
	var kept []repository.TypeEdgeRec
	adj := map[string][]string{}
	for _, e := range edges {
		if !anyAllowed(e.Types, allowed) {
			continue
		}
		kept = append(kept, e)
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	nsOf, err := ix.repo.ElementNamespaces()
	if err != nil {
		return err
	}

	groupID := 0
	for _, comp := range tarjan(adj) {
		if len(comp) < 2 {
			continue
		}
		pkgs := map[string]bool{}
		for _, m := range comp {
			pkgs[namespaceOf(nsOf, m)] = true
		}
		if len(pkgs) < 2 {
			continue // 包内环不计算
		}
		groupID++

		memberSet := map[string]bool{}
		members := make([]repository.CycleMemberRow, 0, len(comp))
		for _, m := range comp {
			memberSet[m] = true
			members = append(members, repository.CycleMemberRow{
				GroupID: groupID, TypeQN: m, NamespaceQN: namespaceOf(nsOf, m),
			})
		}
		var hops []repository.CycleEdgeRow
		for _, e := range kept {
			if !memberSet[e.Source] || !memberSet[e.Target] {
				continue
			}
			hops = append(hops, repository.CycleEdgeRow{
				GroupID: groupID, SourceQN: e.Source, TargetQN: e.Target, Types: e.Types, Sites: e.Sites,
			})
		}
		if err := ix.repo.InsertCycleGroup(dimID, groupID, len(comp), len(pkgs), members, hops); err != nil {
			return err
		}
	}
	return nil
}

// namespaceOf 返回类型的所属命名空间（空则默认包）。
func namespaceOf(nsOf map[string]string, qn string) string {
	if ns := nsOf[qn]; ns != "" {
		return ns
	}
	return DefaultNamespace
}

// anyAllowed 判断关系类型集合与白名单是否有交集。
func anyAllowed(types, allowed []string) bool {
	set := map[string]bool{}
	for _, a := range allowed {
		set[a] = true
	}
	for _, t := range types {
		if set[t] {
			return true
		}
	}
	return false
}

// buildPkgRollup 包级上卷（P9）：沿 parent_qn 自底向上累加。
func (ix *Preprocessor) buildPkgRollup() error {
	dimID := ix.dim.ID
	nsLevel := ix.dim.LevelOfRole(RoleNamespace)
	typeLevel := ix.dim.LevelOfRole(RoleType)

	if err := ix.repo.BuildPkgRollup(dimID, nsLevel, typeLevel); err != nil {
		return err
	}
	if err := ix.repo.RollupColumn(dimID, nsLevel, "subtree_type_count", "direct_type_count"); err != nil {
		return err
	}
	return ix.rollupViolations(dimID, nsLevel, typeLevel)
}

// rollupViolations 违规命中与环组按宿主包向祖先链累加。
func (ix *Preprocessor) rollupViolations(dimID, nsLevel, typeLevel int) error {
	typePkg, err := ix.repo.ParentPairs(dimID, typeLevel)
	if err != nil {
		return err
	}
	parentOf, err := ix.repo.ParentPairs(dimID, nsLevel)
	if err != nil {
		return err
	}
	ancestors := func(pkg string) []string {
		var out []string
		seen := map[string]bool{}
		for cur := pkg; cur != "" && !seen[cur]; cur = parentOf[cur] {
			seen[cur] = true
			out = append(out, cur)
		}
		return out
	}

	// 违规命中：按 metric 累加。
	for _, metric := range []struct{ name, dst string }{
		{"fanin", "subtree_high_fanin"},
		{"fanout", "subtree_high_fanout"},
	} {
		qns, err := ix.repo.WarnHitQNs(dimID, metric.name)
		if err != nil {
			return err
		}
		for _, qn := range qns {
			pkg := typePkg[qn]
			if pkg == "" {
				continue
			}
			for _, a := range ancestors(pkg) {
				if err := ix.repo.IncrementRollup(dimID, metric.dst, a); err != nil {
					return err
				}
			}
		}
	}

	// 环组：同一祖先只计一次。
	groupMembers, err := ix.repo.CycleMembersByGroup(dimID)
	if err != nil {
		return err
	}
	for _, members := range groupMembers {
		seen := map[string]bool{}
		for _, qn := range members {
			if pkg := typePkg[qn]; pkg != "" {
				for _, a := range ancestors(pkg) {
					seen[a] = true
				}
			}
		}
		for a := range seen {
			if err := ix.repo.IncrementRollup(dimID, "subtree_cycle_groups", a); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildMetrics 规模指标（P12，10 项）。
func (ix *Preprocessor) buildMetrics() error {
	dimID := ix.dim.ID
	typeKinds := ix.dim.KindsOfLevel(ix.dim.LevelOfRole(RoleType))
	kinds := make([]string, 0, len(typeKinds))
	for k := range typeKinds {
		kinds = append(kinds, k)
	}
	return ix.repo.BuildMetrics(dimID,
		ix.dim.LevelOfRole(RoleNamespace), ix.dim.LevelOfRole(RoleType), kinds)
}
