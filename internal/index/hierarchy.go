package index

import (
	"fmt"

	"github.com/CodMac/arch-lens-dep-displayer/internal/dimension"
)

// buildHierarchy 依据维度配置构建层级树并写回 hierarchy_node / element_node。
// 规则（维度驱动，kind 白名单来自 dimension.Hierarchy）：
//   - PACKAGE 层：遍历全部元素 QN，按 "." 逐级拆出包节点（com → com.demo → ...）
//   - CLASS 层：kind ∈ 维度 level2.parent_of 的元素（CLASS/INTERFACE/ENUM）归属其包
//   - MEMBER 层：kind ∈ level3.parent_of 的元素（METHOD/FIELD）从 QN 提取宿主类归属
func (ix *Indexer) buildHierarchy() error {
	db := ix.db
	dim := ix.dim
	dimID := dim.ID

	// 1. 清空该维度的旧层级数据（重复导入全量重建）。
	for _, q := range []string{
		`DELETE FROM hierarchy_node WHERE dimension_id = ?`,
		`DELETE FROM element_node WHERE dimension_id = ?`,
	} {
		if _, err := db.Exec(q, dimID); err != nil {
			return fmt.Errorf("clear hierarchy: %w", err)
		}
	}

	// 2. 读全部元素（qn, kind）。
	rows, err := db.Query(`SELECT qn, kind FROM elements`)
	if err != nil {
		return fmt.Errorf("query elements: %w", err)
	}
	defer rows.Close()

	// 收集各层元素。
	var pkgs = map[string]bool{} // 包节点集合
	var classEls []elmRef        // level2 元素（类/接口/枚举）
	var memberEls []elmRef       // level3 元素（方法/字段）
	explicitPkgs := []string{}   // 显式 PACKAGE 元素 QN（已知包）

	classKinds := kindSet(dim, 2)
	memberKinds := kindSet(dim, 3)

	// 第一遍：先收集显式 PACKAGE 元素，作为包边界判定依据。
	for rows.Next() {
		var qn, kind string
		if err := rows.Scan(&qn, &kind); err != nil {
			return err
		}
		if qn == "" {
			continue
		}
		if kind == "PACKAGE" {
			explicitPkgs = append(explicitPkgs, qn)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// 包链：显式包前缀逐级加入。
	for _, p := range explicitPkgs {
		for _, seg := range splitQN(p) {
			pkgs[seg] = true
		}
	}
	// 重开游标扫描其余元素（DuckDB 单连接下需重新查询）。
	rows.Close()
	rows2, err := db.Query(`SELECT qn, kind FROM elements`)
	if err != nil {
		return fmt.Errorf("query elements: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var qn, kind string
		if err := rows2.Scan(&qn, &kind); err != nil {
			return err
		}
		if qn == "" {
			continue
		}
		if classKinds[kind] {
			classEls = append(classEls, elmRef{qn, kind})
			// 类归属包：取已知包边界（显式包集合内最长前缀），兜底逐级拆。
			addClassPackage(pkgs, qn, explicitPkgs)
		} else if memberKinds[kind] {
			memberEls = append(memberEls, elmRef{qn, kind})
		}
		// FILE 等其他元素不参与包链（其 QN 可能是文件路径风格，勿误解析）。
	}
	if err := rows2.Err(); err != nil {
		return err
	}

	// 3. 写包层级树（PACKAGE 层，含子包边）。
	//    包节点 QN 即包名；父 = 去掉最后一段。
	if _, err := db.Exec(`INSERT INTO hierarchy_node (dimension_id, qn, level, parent_qn, kind) VALUES (?,?,?,?,?)`,
		dimID, "", 0, "", "ROOT"); err != nil {
		return err
	}
	for pkg := range pkgs {
		parent := parentPackage(pkg)
		if _, err := db.Exec(`INSERT INTO hierarchy_node (dimension_id, qn, level, parent_qn, kind) VALUES (?,?,?,?,?)`,
			dimID, pkg, 1, parent, "PACKAGE"); err != nil {
			return err
		}
	}

	// 4. 写类层级节点 + 元素归属。
	for _, ce := range classEls {
		pkg := classPackage(ce.qn, explicitPkgs)
		if _, err := db.Exec(`INSERT INTO hierarchy_node (dimension_id, qn, level, parent_qn, kind) VALUES (?,?,?,?,?)`,
			dimID, ce.qn, 2, pkg, ce.kind); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT INTO element_node (element_qn, dimension_id, node_qn, level) VALUES (?,?,?,?)`,
			ce.qn, dimID, ce.qn, 2); err != nil {
			return err
		}
	}

	// 5. 写成员归属（MEMBER 层不建 hierarchy_node，只建 element_node 指向宿主类）。
	for _, me := range memberEls {
		host := hostClass(me.qn)
		if host == "" {
			continue
		}
		if _, err := db.Exec(`INSERT INTO element_node (element_qn, dimension_id, node_qn, level) VALUES (?,?,?,?)`,
			me.qn, dimID, host, 3); err != nil {
			return err
		}
	}

	return nil
}

// elmRef 元素引用。
type elmRef struct {
	qn   string
	kind string
}

// kindSet 返回维度某层级的 kind 白名单。
func kindSet(dim *dimension.Dimension, level int) map[string]bool {
	s := map[string]bool{}
	for _, l := range dim.Hierarchy {
		if l.Level != level {
			continue
		}
		for _, k := range l.ParentOf {
			s[k] = true
		}
		// 自身 kind 也接受（如 CLASS 层节点 kind 为 CLASS/INTERFACE/ENUM）
		s[l.Kind] = true
	}
	return s
}
