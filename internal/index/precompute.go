package index

import (
	"fmt"
	"strings"
)

// precompute 执行全部预计算：level_edges / fan / scc / metrics。
func (ix *Indexer) precompute() error {
	db := ix.db
	dimID := ix.dim.ID

	// 清空旧数据。
	for _, q := range []string{
		`DELETE FROM level_edges WHERE dimension_id = ?`,
		`DELETE FROM fan WHERE dimension_id = ?`,
		`DELETE FROM scc WHERE dimension_id = ?`,
		`DELETE FROM metrics WHERE dimension_id = ?`,
	} {
		if _, err := db.Exec(q, dimID); err != nil {
			return fmt.Errorf("clear precompute: %w", err)
		}
	}

	// 1. level_edges：底层关系 → 按 (level, 源节点, 目标节点) 聚合。
	//    关系 source/target QN 归属到当前维度下最近层级节点（class 级），
	//    再由 class 级归属到包级（level 聚合由 SQL 逐级上卷完成）。
	if err := ix.buildLevelEdges(); err != nil {
		return err
	}

	// 2. fan：由 level_edges 推导（level=2 类级扇入扇出）。
	if err := ix.buildFan(); err != nil {
		return err
	}

	// 3. scc：以类级节点为节点、类级关系为边，Tarjan 求强连通分量。
	if err := ix.buildSCC(); err != nil {
		return err
	}

	// 4. metrics：规模指标。
	if err := ix.buildMetrics(); err != nil {
		return err
	}

	return nil
}

// buildLevelEdges 聚合层级边。
// 方案 D（同画布双层）：依赖边与 CONTAIN 背景边分离。
//
//	依赖边（CALL/CREATE/EXTEND/...）：relations 中非 CONTAIN 关系，两端归属到最近层级节点；
//	  剔除无 element_node 且非外部的语法端点（VARIABLE/SCOPE_BLOCK 等不上图）。
//	CONTAIN 背景边：由 hierarchy_node/element_node 层级树推导（包→包、包→类、类→成员），
//	  天然只含真实层级节点，不含语法作用域嵌套。
func (ix *Indexer) buildLevelEdges() error {
	db := ix.db
	dimID := ix.dim.ID

	// 4a. 类级依赖边（level=2）：非 CONTAIN 关系，两端归属到宿主类。
	//     端点合法判定：有 element_node 归属（成员/类）或 在 hierarchy_node（类级节点）或 外部类型。
	//     三者皆无者（VARIABLE/SCOPE_BLOCK 等语法元素）剔除。
	//     子查询外层过滤 NULL（DuckDB 的 HAVING 不支持引用 SELECT 别名）。
	q := `INSERT INTO level_edges (dimension_id, level, source_qn, target_qn, edge_count, types)
		SELECT dimension_id, level, source_qn, target_qn, edge_count, types FROM (
			SELECT ? AS dimension_id, 2 AS level,
			       CASE WHEN sn.node_qn IS NULL AND hn_s.qn IS NULL AND NOT COALESCE(e_source.is_external, false) THEN NULL
			            ELSE COALESCE(sn.node_qn, r.source_qn) END AS source_qn,
			       CASE WHEN tn.node_qn IS NULL AND hn_t.qn IS NULL AND NOT COALESCE(e_target.is_external, false) THEN NULL
			            ELSE COALESCE(tn.node_qn, r.target_qn) END AS target_qn,
			       COUNT(*) AS edge_count,
			       string_agg(DISTINCT r.type, ',') AS types
			FROM relations r
			LEFT JOIN element_node sn ON sn.element_qn = r.source_qn AND sn.dimension_id = ?
			LEFT JOIN element_node tn ON tn.element_qn = r.target_qn AND tn.dimension_id = ?
			LEFT JOIN hierarchy_node hn_s ON hn_s.qn = r.source_qn AND hn_s.dimension_id = ?
			LEFT JOIN hierarchy_node hn_t ON hn_t.qn = r.target_qn AND hn_t.dimension_id = ?
			LEFT JOIN elements e_source ON e_source.qn = r.source_qn
			LEFT JOIN elements e_target ON e_target.qn = r.target_qn
			WHERE r.type <> 'CONTAIN'
			GROUP BY 1,2,3,4
		) WHERE source_qn IS NOT NULL AND target_qn IS NOT NULL`
	if _, err := db.Exec(q, dimID, dimID, dimID, dimID, dimID); err != nil {
		return fmt.Errorf("level_edges class-level: %w", err)
	}

	// 4b. 包级依赖边（level=1）：由类级依赖边 source/target 各取包前缀上卷（排除 CONTAIN 背景边）。
	q1 := `INSERT INTO level_edges (dimension_id, level, source_qn, target_qn, edge_count, types)
		SELECT ?,
		       1,
		       pkg_of(e.source_qn),
		       pkg_of(e.target_qn),
		       SUM(e.edge_count),
		       string_agg(DISTINCT e.types, ',')
		FROM level_edges e
		WHERE e.dimension_id = ? AND e.level = 2 AND e.types <> 'CONTAIN'
		GROUP BY 1,2,3,4`
	// pkg_of 在 DuckDB 中无内置；用 SQL 表达式：去掉最后一段。
	q1 = strings.Replace(q1, "pkg_of(e.source_qn)", pkgExpr("e.source_qn"), 1)
	q1 = strings.Replace(q1, "pkg_of(e.target_qn)", pkgExpr("e.target_qn"), 1)
	if _, err := db.Exec(q1, dimID, dimID); err != nil {
		return fmt.Errorf("level_edges package-level: %w", err)
	}

	// 4c. CONTAIN 背景边：由层级树推导（不含语法作用域嵌套）。
	//     类←包（level2 节点的 parent_qn）、成员←类（element_node.level=3）。
	q2 := `INSERT INTO level_edges (dimension_id, level, source_qn, target_qn, edge_count, types)
		SELECT ?, 2, parent_qn, qn, 1, 'CONTAIN' FROM hierarchy_node
		WHERE dimension_id = ? AND level = 2 AND parent_qn <> ''
		UNION ALL
		SELECT ?, 2, en.node_qn, en.element_qn, 1, 'CONTAIN' FROM element_node en
		WHERE en.dimension_id = ? AND en.level = 3`
	if _, err := db.Exec(q2, dimID, dimID, dimID, dimID); err != nil {
		return fmt.Errorf("level_edges contain background: %w", err)
	}

	return nil
}

// pkgExpr 生成取包前缀的 SQL 表达式（去最后一段）。
func pkgExpr(col string) string {
	return fmt.Sprintf("CASE WHEN instr(%[1]s, '.') > 0 THEN substr(%[1]s, 1, length(%[1]s) - instr(reverse(%[1]s), '.') ) ELSE %[1]s END", col)
}

// buildFan 由 level=2 的类级依赖边统计扇入扇出（剔除 CONTAIN 背景边）。
func (ix *Indexer) buildFan() error {
	db := ix.db
	dimID := ix.dim.ID
	q := `INSERT INTO fan (dimension_id, qn, fan_in, fan_out)
		SELECT ?,
		       node,
		       SUM(fan_in),
		       SUM(fan_out)
		FROM (
		    SELECT source_qn AS node, 0 AS fan_in, COUNT(*) AS fan_out
		    FROM level_edges WHERE dimension_id = ? AND level = 2 AND types <> 'CONTAIN'
		    GROUP BY source_qn
		    UNION ALL
		    SELECT target_qn AS node, COUNT(*) AS fan_in, 0 AS fan_out
		    FROM level_edges WHERE dimension_id = ? AND level = 2 AND types <> 'CONTAIN'
		    GROUP BY target_qn
		)
		GROUP BY node`
	if _, err := db.Exec(q, dimID, dimID, dimID); err != nil {
		return fmt.Errorf("fan: %w", err)
	}
	return nil
}

// buildSCC 以类级边为图跑 Tarjan，写回 size>1 的强连通分量。
func (ix *Indexer) buildSCC() error {
	db := ix.db
	dimID := ix.dim.ID
	rows, err := db.Query(`SELECT source_qn, target_qn FROM level_edges WHERE dimension_id = ? AND level = 2 AND source_qn <> target_qn AND types <> 'CONTAIN'`, dimID)
	if err != nil {
		return fmt.Errorf("scc read edges: %w", err)
	}
	defer rows.Close()

	adj := map[string][]string{}
	for rows.Next() {
		var s, t string
		if err := rows.Scan(&s, &t); err != nil {
			return err
		}
		adj[s] = append(adj[s], t)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	comps := tarjan(adj)
	cid := 0
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, comp := range comps {
		if len(comp) < 2 {
			continue
		}
		cid++
		for _, m := range comp {
			if _, err := tx.Exec(`INSERT INTO scc (dimension_id, component_id, member_qn) VALUES (?,?,?)`, dimID, cid, m); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// buildMetrics 统计规模指标。
func (ix *Indexer) buildMetrics() error {
	db := ix.db
	dimID := ix.dim.ID
	q := `INSERT INTO metrics (dimension_id, kind, count)
		SELECT ?, 'element_total', COUNT(*) FROM elements
		UNION ALL
		SELECT ?, 'relation_total', COUNT(*) FROM relations
		UNION ALL
		SELECT ?, 'class_total', COUNT(*) FROM elements WHERE kind IN ('CLASS','INTERFACE','ENUM')
		UNION ALL
		SELECT ?, 'package_total', COUNT(*) FROM (SELECT DISTINCT qn FROM hierarchy_node WHERE dimension_id = ? AND level = 1)`
	if _, err := db.Exec(q, dimID, dimID, dimID, dimID, dimID); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	return nil
}
