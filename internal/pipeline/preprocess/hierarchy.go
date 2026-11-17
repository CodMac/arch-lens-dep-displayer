package preprocess

import (
	"fmt"
	"sort"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// DefaultNamespace 默认包 QN。
// analyzer 已为无 package 声明的元素输出 NamespaceQN="(default)" 并注册 PACKAGE 元素，
// 此处保留同值常量用于：C++ 等「namespace_qn 为空是正常语义」的语言做展示兜底，
// 以及兼容旧数据（analyzer 修复前导出的 namespace_qn 为空）。
const DefaultNamespace = "(default)"

// buildHierarchy 依据维度配置构建层级树并写回 hierarchy_node / element_node。
// 规则（维度驱动，kind 白名单来自 dimension.Hierarchy 的 role/kinds）：
//   - role=namespace：包/命名空间/模块节点，父节点取 CONTAIN 关系，兜底按 qn_separator 逐级拆
//   - role=type：类型节点，父节点取 elements.namespace_qn（空则挂 (default)）
//   - role=member：不建 hierarchy_node，只建 element_node，按 type_qn → namespace_qn → 文件节点回退
func (ix *Preprocessor) buildHierarchy() error {
	dim := ix.dim
	dimID := dim.ID

	nsLevel := dim.LevelOfRole(RoleNamespace)
	typeLevel := dim.LevelOfRole(RoleType)
	memberLevel := dim.LevelOfRole(RoleMember)
	if nsLevel == 0 || typeLevel == 0 {
		return fmt.Errorf("dimension %s: namespace/type 层未配置", dim.Language)
	}

	// 1. 清空该维度的旧层级数据（Resume / 单维度重跑时幂等）。
	if err := ix.repo.ClearHierarchy(dimID); err != nil {
		return err
	}

	// 2. 读全部元素。
	all, err := ix.repo.HierarchyElements()
	if err != nil {
		return fmt.Errorf("query elements: %w", err)
	}

	nsKinds := dim.KindsOfLevel(nsLevel)
	typeKinds := dim.KindsOfLevel(typeLevel)
	memberKinds := dim.KindsOfLevel(memberLevel)

	// 3. 收集命名空间节点：显式 namespace 元素 + 元素归属的 namespace_qn。
	nsSet := map[string]bool{}
	for _, e := range all {
		if nsKinds[e.Kind] {
			nsSet[e.QN] = true
		}
		if e.NamespaceQN != "" {
			nsSet[e.NamespaceQN] = true
		}
	}
	// 默认包兜底：存在 namespace_qn 为空的非文件/非命名空间元素 → 挂虚拟节点。
	// analyzer 修复后 Java 默认包元素已自带 NamespaceQN="(default)"（走上面的分支）；
	// 此分支用于兼容旧数据，以及 C++ 等「空 namespace_qn 是正常语义」的语言。
	for _, e := range all {
		if e.NamespaceQN == "" && !nsKinds[e.Kind] && e.Kind != "FILE" {
			nsSet[DefaultNamespace] = true
			break
		}
	}

	// 命名空间父节点：CONTAIN 关系优先（包→包），兜底按分隔符逐级拆。
	nsParent, err := ix.namespaceParents(nsSet, dim.QNSeparator)
	if err != nil {
		return err
	}

	// 4. 写命名空间层级树（深度自顶向下计算）。
	nsList := make([]string, 0, len(nsSet))
	for qn := range nsSet {
		nsList = append(nsList, qn)
	}
	sort.Strings(nsList)
	nsRows := make([]repository.HierarchyRow, 0, len(nsList))
	for _, qn := range nsList {
		nsRows = append(nsRows, repository.HierarchyRow{
			DimensionID: dimID, QN: qn, Level: nsLevel, Role: RoleNamespace,
			ParentQN: nsParent[qn], Kind: "PACKAGE",
		})
	}
	if err := ix.repo.InsertHierarchyNodes(nsRows); err != nil {
		return err
	}
	if err := ix.fillNamespaceDepth(dimID, nsLevel, nsParent); err != nil {
		return err
	}

	// 5. 写类型节点 + 元素归属。
	//    嵌套类型归并：type_qn 指向宿主且不是自身 → 不单独建节点（G5）。
	typeNodeSet := map[string]bool{}
	var typeRows []repository.HierarchyRow
	var typeElemRows []repository.ElementNodeRow
	for _, e := range all {
		if !typeKinds[e.Kind] {
			continue
		}
		if e.TypeQN != "" && e.TypeQN != e.QN {
			continue
		}
		parent := e.NamespaceQN
		if parent == "" {
			parent = DefaultNamespace
		}
		typeRows = append(typeRows, repository.HierarchyRow{
			DimensionID: dimID, QN: e.QN, Level: typeLevel, Role: RoleType, ParentQN: parent, Kind: e.Kind,
		})
		typeNodeSet[e.QN] = true
		typeElemRows = append(typeElemRows, repository.ElementNodeRow{
			ElementQN: e.QN, ElementKind: e.Kind, DimensionID: dimID, NodeQN: e.QN, Level: typeLevel,
		})
	}
	if err := ix.repo.InsertHierarchyNodes(typeRows); err != nil {
		return err
	}
	if err := ix.repo.InsertElementNodes(typeElemRows); err != nil {
		return err
	}

	// 6. 写成员归属（member 层不建 hierarchy_node，只建 element_node）。
	//    回退链：type_qn → namespace_qn → 文件节点（决策 11）。
	fileNode := map[string]string{} // path → 文件节点 QN
	for _, e := range all {
		if e.Kind == "FILE" {
			fileNode[e.Path] = e.QN
		}
	}
	var memberRows []repository.ElementNodeRow
	for _, e := range all {
		if !memberKinds[e.Kind] {
			continue
		}
		nodeQN, level := "", 0
		if e.TypeQN != "" && typeNodeSet[e.TypeQN] {
			nodeQN, level = e.TypeQN, typeLevel
		}
		if nodeQN == "" && e.NamespaceQN != "" {
			nodeQN, level = e.NamespaceQN, nsLevel
		}
		if nodeQN == "" && e.TypeQN == "" && e.NamespaceQN == "" {
			nodeQN, level = DefaultNamespace, nsLevel
		}
		if nodeQN == "" {
			if fq, ok := fileNode[e.Path]; ok {
				nodeQN, level = fq, nsLevel
			}
		}
		if nodeQN == "" {
			continue
		}
		memberRows = append(memberRows, repository.ElementNodeRow{
			ElementQN: e.QN, ElementKind: e.Kind, DimensionID: dimID, NodeQN: nodeQN, Level: level,
		})
	}
	return ix.repo.InsertElementNodes(memberRows)
}

// namespaceParents 计算命名空间父节点：CONTAIN 关系优先，兜底按分隔符逐级拆。
func (ix *Preprocessor) namespaceParents(nsSet map[string]bool, sep string) (map[string]string, error) {
	parent := map[string]string{}
	contain, err := ix.repo.ContainParents()
	if err != nil {
		return nil, err
	}
	for child, par := range contain {
		if nsSet[child] && nsSet[par] {
			parent[child] = par
		}
	}
	for qn := range nsSet {
		if _, ok := parent[qn]; ok || qn == DefaultNamespace {
			continue
		}
		parent[qn] = parentBySeparator(qn, nsSet, sep)
	}
	return parent, nil
}

// parentBySeparator 返回 qn 在 nsSet 中的最近祖先（按分隔符逐级截断），无则 ""。
func parentBySeparator(qn string, nsSet map[string]bool, sep string) string {
	cur := qn
	for {
		i := lastIndexSep(cur, sep)
		if i <= 0 {
			return ""
		}
		cur = cur[:i]
		if nsSet[cur] {
			return cur
		}
	}
}

// lastIndexSep 返回分隔符最后一次出现的位置（支持多字符分隔符如 "::"）。
func lastIndexSep(s, sep string) int {
	if sep == "" {
		return -1
	}
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// fillNamespaceDepth 按 parent_qn 链计算命名空间深度（顶层=0）。
func (ix *Preprocessor) fillNamespaceDepth(dimID, nsLevel int, parent map[string]string) error {
	depth := map[string]int{}
	var resolve func(qn string, guard map[string]bool) int
	resolve = func(qn string, guard map[string]bool) int {
		if d, ok := depth[qn]; ok {
			return d
		}
		if guard[qn] {
			return 0
		}
		guard[qn] = true
		d := 0
		if p := parent[qn]; p != "" {
			d = resolve(p, guard) + 1
		}
		depth[qn] = d
		return d
	}
	for qn := range parent {
		resolve(qn, map[string]bool{})
	}
	for qn, d := range depth {
		if err := ix.repo.UpdateHierarchyDepth(dimID, nsLevel, qn, d); err != nil {
			return err
		}
	}
	return nil
}
