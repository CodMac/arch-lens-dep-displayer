package repository

import (
	"database/sql"
	"strings"
)

// Node 图节点。Size=直接子节点数（规模）。
type Node struct {
	ID    string
	Label string
	Kind  string
	Size  int
}

// Edge 图边。Type 为主类型，Types 为完整类型集。
type Edge struct {
	Source string
	Target string
	Type   string
	Types  []string
	Count  int
}

// Dimension 维度及其层级。
type Dimension struct {
	ID       int
	Language string
	Name     string
	Levels   []Level
}

// Level 维度层级。
type Level struct {
	Level    int
	Kind     string
	ParentOf []string
}

// Dimensions 列出全部维度（含层级定义）。
func (r *Repository) Dimensions() ([]Dimension, error) {
	rows, err := r.db().Query(`SELECT id, language, name FROM dimensions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dims := make([]Dimension, 0)
	for rows.Next() {
		var d Dimension
		if err := rows.Scan(&d.ID, &d.Language, &d.Name); err != nil {
			return nil, err
		}
		dims = append(dims, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range dims {
		lrows, err := r.db().Query(`SELECT level, kind, parent_of FROM dimension_levels WHERE dimension_id = ? ORDER BY level`, dims[i].ID)
		if err != nil {
			return nil, err
		}
		for lrows.Next() {
			var lv Level
			var po string
			if err := lrows.Scan(&lv.Level, &lv.Kind, &po); err != nil {
				lrows.Close()
				return nil, err
			}
			if po != "" {
				lv.ParentOf = strings.Split(po, ",")
			}
			dims[i].Levels = append(dims[i].Levels, lv)
		}
		lrows.Close()
	}
	return dims, nil
}

// TopNodes 返回指定层级顶层节点（size=直接子节点数）。
func (r *Repository) TopNodes(dimID, level int) ([]Node, error) {
	rows, err := r.db().Query(`SELECT qn, kind,
			(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = h.dimension_id AND c.level = h.level + 1 AND c.parent_qn = h.qn)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = ? ORDER BY qn`, dimID, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Kind, &n.Size); err != nil {
			return nil, err
		}
		n.Label = lastSeg(n.ID)
		nodes = append(nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// OverviewNodes 返回 level=1 顶层包节点（size=包内直接类数）。
func (r *Repository) OverviewNodes(dimID int) ([]Node, error) {
	rows, err := r.db().Query(`SELECT h.qn, h.kind,
			(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = h.dimension_id AND c.level = h.level + 1 AND c.parent_qn = h.qn)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = 1 ORDER BY h.qn`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Kind, &n.Size); err != nil {
			return nil, err
		}
		n.Label = lastSeg(n.ID)
		nodes = append(nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// LevelEdges 返回指定层级全部聚合边。
// 包级（level=1）：只保留两端都是真实层级节点的依赖边，剔除纯 CONTAIN（层级嵌套非依赖）
// 与 FILE 伪节点泄漏边，保证「包间聚合边宽=依赖条数」语义纯净。
func (r *Repository) LevelEdges(dimID, level int) ([]Edge, error) {
	depFilter := ""
	if level == 1 {
		depFilter = ` AND e.source_qn IN (SELECT qn FROM hierarchy_node WHERE dimension_id = ? AND level = 1)
			AND e.target_qn IN (SELECT qn FROM hierarchy_node WHERE dimension_id = ? AND level = 1)
			AND e.source_qn <> e.target_qn
			AND NOT (e.types = 'CONTAIN')`
	}
	q := `SELECT e.source_qn, e.target_qn, e.types, e.edge_count FROM level_edges e
		WHERE e.dimension_id = ? AND e.level = ?` + depFilter + ` ORDER BY e.source_qn, e.target_qn`
	var rows *sql.Rows
	var err error
	if level == 1 {
		rows, err = r.db().Query(q, dimID, level, dimID, dimID)
	} else {
		rows, err = r.db().Query(q, dimID, level)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var typesStr string
		if err := rows.Scan(&e.Source, &e.Target, &typesStr, &e.Count); err != nil {
			return nil, err
		}
		e.Types = SplitTypes(typesStr)
		if len(e.Types) > 0 {
			e.Type = e.Types[0]
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// NodeLevel 返回节点 qn 所属层级（hierarchy_node 优先，element_node 兜底）。
// 找不到返回 0, false。
func (r *Repository) NodeLevel(dimID int, qn string) (int, bool) {
	var level int
	if err := r.db().QueryRow(`SELECT level FROM hierarchy_node WHERE dimension_id = ? AND qn = ?`, dimID, qn).Scan(&level); err == nil {
		return level, true
	}
	if err := r.db().QueryRow(`SELECT level FROM element_node WHERE dimension_id = ? AND node_qn = ? LIMIT 1`, dimID, qn).Scan(&level); err == nil {
		return level, true
	}
	return 0, false
}

// ChildNodes 返回节点 qn 的下一层子节点（hierarchy_node 优先，element_node 成员兜底）。
func (r *Repository) ChildNodes(dimID int, qn string, level, nextLevel int) ([]Node, error) {
	rows, err := r.db().Query(`SELECT h.qn, h.kind,
			(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = h.dimension_id AND c.level = h.level + 1 AND c.parent_qn = h.qn)
			+ (SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = h.dimension_id AND en.node_qn = h.qn AND en.level = h.level + 1)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = ? AND h.parent_qn = ? ORDER BY h.qn`, dimID, nextLevel, qn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Kind, &n.Size); err != nil {
			return nil, err
		}
		n.Label = lastSeg(n.ID)
		nodes = append(nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 若层级节点为空，补充 element_node 归属的下一层元素（如类下的成员）。
	if len(nodes) == 0 {
		elemRows, err := r.db().Query(`SELECT DISTINCT e.qn, e.kind FROM element_node en JOIN elements e ON e.qn = en.element_qn WHERE en.dimension_id = ? AND en.node_qn = ? AND en.level = ? ORDER BY e.qn`, dimID, qn, nextLevel)
		if err != nil {
			return nil, err
		}
		defer elemRows.Close()
		for elemRows.Next() {
			var n Node
			if err := elemRows.Scan(&n.ID, &n.Kind); err != nil {
				continue
			}
			n.Label = lastSeg(n.ID)
			n.Size = 0
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// NodeSize 返回节点直接子节点数（hierarchy_node 子层 + element_node 成员）。
func (r *Repository) NodeSize(dimID int, qn string, level int) int {
	var size int
	err := r.db().QueryRow(`SELECT
			(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = ? AND c.level = ? + 1 AND c.parent_qn = ?)
			+ (SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = ? AND en.node_qn = ? AND en.level = ? + 1)`, dimID, level, qn, dimID, qn, level).Scan(&size)
	if err != nil {
		return 0
	}
	return size
}

// KindOf 查节点 kind（hierarchy_node 或 elements）。
func (r *Repository) KindOf(dimID int, qn string) string {
	var k string
	if err := r.db().QueryRow(`SELECT kind FROM hierarchy_node WHERE dimension_id = ? AND qn = ?`, dimID, qn).Scan(&k); err == nil {
		return k
	}
	if err := r.db().QueryRow(`SELECT kind FROM elements WHERE qn = ?`, qn).Scan(&k); err == nil {
		return k
	}
	return "UNKNOWN"
}

// FocusEdges 返回以 qn 为源或目标的层级边（可选类型过滤）。
func (r *Repository) FocusEdges(dimID, level int, qn string, types []string) ([]Edge, error) {
	typeSQL := ""
	var args []interface{}
	if len(types) > 0 {
		typeSQL = " AND types IN (" + strings.Repeat("?,", len(types)-1) + "?)"
		for _, t := range types {
			args = append(args, t)
		}
	}
	rows, err := r.db().Query(`SELECT source_qn, target_qn, types, edge_count FROM level_edges
		WHERE dimension_id = ? AND level = ? AND (source_qn = ? OR target_qn = ?)`+typeSQL,
		append([]interface{}{dimID, level, qn, qn}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var typesStr string
		if err := rows.Scan(&e.Source, &e.Target, &typesStr, &e.Count); err != nil {
			return nil, err
		}
		e.Types = SplitTypes(typesStr)
		if len(e.Types) > 0 {
			e.Type = e.Types[0]
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// FanEdges 返回节点 qn 的入边或出边（level 层级）。isIn=true 入边。
func (r *Repository) FanEdges(dimID, level int, qn string, isIn bool) ([]Edge, error) {
	var rows *sql.Rows
	var err error
	if isIn {
		rows, err = r.db().Query(`SELECT source_qn, target_qn, types, edge_count FROM level_edges
			WHERE dimension_id = ? AND level = ? AND target_qn = ?`, dimID, level, qn)
	} else {
		rows, err = r.db().Query(`SELECT source_qn, target_qn, types, edge_count FROM level_edges
			WHERE dimension_id = ? AND level = ? AND source_qn = ?`, dimID, level, qn)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var typesStr string
		if err := rows.Scan(&e.Source, &e.Target, &typesStr, &e.Count); err != nil {
			return nil, err
		}
		e.Types = SplitTypes(typesStr)
		if len(e.Types) > 0 {
			e.Type = e.Types[0]
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// SplitTypes 拆分 level_edges.types 逗号串为去重有序数组。
func SplitTypes(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// lastSeg 返回 QN 最后一段（label 用）。
func lastSeg(qn string) string {
	if i := strings.LastIndex(qn, "."); i >= 0 {
		return qn[i+1:]
	}
	return qn
}
