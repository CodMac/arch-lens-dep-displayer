package repository

import (
	"database/sql"
)

// HierarchyRow 层级节点写入行。
type HierarchyRow struct {
	DimensionID int
	QN          string
	Level       int
	Role        string
	ParentQN    string
	Depth       int
	Kind        string
}

// InsertHierarchyNodes 批量写入层级节点。
func (r *Repository) InsertHierarchyNodes(rows []HierarchyRow) error {
	data := make([][]interface{}, 0, len(rows))
	for _, n := range rows {
		data = append(data, []interface{}{n.DimensionID, n.QN, n.Level, n.Role, n.ParentQN, n.Depth, n.Kind})
	}
	return r.insertBatches(`INSERT INTO hierarchy_node (dimension_id, qn, level, role, parent_qn, depth, kind)`, 7, data)
}

// ClearHierarchy 清空某维度的层级表（hierarchy_node / element_node）。
func (r *Repository) ClearHierarchy(dimID int) error {
	for _, q := range []string{
		`DELETE FROM hierarchy_node WHERE dimension_id = ?`,
		`DELETE FROM element_node WHERE dimension_id = ?`,
	} {
		if _, err := r.db().Exec(q, dimID); err != nil {
			return err
		}
	}
	return nil
}

// UpdateHierarchyDepth 回填单个层级节点的深度。
func (r *Repository) UpdateHierarchyDepth(dimID, level int, qn string, depth int) error {
	_, err := r.db().Exec(`UPDATE hierarchy_node SET depth = ? WHERE dimension_id = ? AND level = ? AND qn = ?`,
		depth, dimID, level, qn)
	return err
}

// ParentPairs 返回某层级的 qn → parent_qn 映射。
func (r *Repository) ParentPairs(dimID, level int) (map[string]string, error) {
	rows, err := r.db().Query(`SELECT qn, COALESCE(parent_qn,'') FROM hierarchy_node
		WHERE dimension_id = ? AND level = ?`, dimID, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var qn, parent string
		if err := rows.Scan(&qn, &parent); err != nil {
			return nil, err
		}
		out[qn] = parent
	}
	return out, rows.Err()
}

// ParentPairsByDepthDesc 返回某层级按 depth 降序的 (qn, parent) 列表（自底向上上卷用）。
func (r *Repository) ParentPairsByDepthDesc(dimID, level int) ([]PkgTree, error) {
	rows, err := r.db().Query(`SELECT qn, COALESCE(parent_qn,''), COALESCE(depth,0)
		FROM hierarchy_node WHERE dimension_id = ? AND level = ? ORDER BY depth DESC`, dimID, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PkgTree
	for rows.Next() {
		var t PkgTree
		if err := rows.Scan(&t.QN, &t.Parent, &t.Depth); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// OrphanNode 层级孤儿（parent_qn 指向不存在的节点）。
type OrphanNode struct {
	QN     string
	Kind   string
	Level  int
	Parent string
}

// OrphanHierarchyNodes 返回 parent_qn 非空但父节点不存在的层级节点（总数 + 明细）。
func (r *Repository) OrphanHierarchyNodes(dimID, limit int) (int, []OrphanNode, error) {
	const cond = `h.parent_qn <> '' AND NOT EXISTS (
		SELECT 1 FROM hierarchy_node p WHERE p.dimension_id = h.dimension_id AND p.qn = h.parent_qn)`
	var total int
	if err := r.db().QueryRow(`SELECT COUNT(*) FROM hierarchy_node h WHERE h.dimension_id = ? AND `+cond, dimID).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := r.db().Query(`SELECT h.qn, h.kind, h.level, h.parent_qn FROM hierarchy_node h
		WHERE h.dimension_id = ? AND `+cond+` ORDER BY h.level, h.qn LIMIT ?`, dimID, limit)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()
	var out []OrphanNode
	for rows.Next() {
		var n OrphanNode
		if err := rows.Scan(&n.QN, &n.Kind, &n.Level, &n.Parent); err != nil {
			return total, out, err
		}
		out = append(out, n)
	}
	return total, out, rows.Err()
}

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

func (r *Repository) OverviewNodes(dimID int) ([]Node, error) {
	tree := r.pkgTree(dimID)
	nsLevel := r.LevelOfRole(dimID, "namespace")

	rows, err := r.db().Query(`SELECT qn, kind FROM hierarchy_node
		WHERE dimension_id = ? AND level = ? ORDER BY qn`, dimID, nsLevel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Kind); err != nil {
			return nil, err
		}
		n.Label = lastSeg(n.ID)
		n.Size = tree.typeCount[n.ID]
		n.Children = len(tree.children[n.ID])
		n.Depth = tree.depth[n.ID]
		nodes = append(nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

type PkgTree struct {
	QN     string
	Parent string
	Depth  int
}

func (r *Repository) PkgTreeStructure(dimID int) ([]PkgTree, error) {
	nsLevel := r.LevelOfRole(dimID, "namespace")
	rows, err := r.db().Query(`SELECT qn, COALESCE(parent_qn,''), COALESCE(depth,0)
		FROM hierarchy_node WHERE dimension_id = ? AND level = ? ORDER BY qn`, dimID, nsLevel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PkgTree
	for rows.Next() {
		var t PkgTree
		if err := rows.Scan(&t.QN, &t.Parent, &t.Depth); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) NodeLevel(dimID int, qn string) (int, bool) {
	var level int
	if err := r.db().QueryRow(`SELECT level FROM hierarchy_node WHERE dimension_id = ? AND qn = ? LIMIT 1`, dimID, qn).Scan(&level); err == nil {
		return level, true
	}
	if err := r.db().QueryRow(`SELECT level FROM element_node WHERE dimension_id = ? AND node_qn = ? LIMIT 1`, dimID, qn).Scan(&level); err == nil {
		return level, true
	}
	return 0, false
}

func (r *Repository) ChildNodes(dimID int, qn string, level, nextLevel int) ([]Node, error) {
	nsLevel := r.LevelOfRole(dimID, "namespace")
	if level == nsLevel {
		return r.pkgChildren(dimID, qn)
	}

	rows, err := r.db().Query(`SELECT h.qn, h.kind,
			(SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = h.dimension_id AND en.node_qn = h.qn AND en.level = h.level + 1)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = ? AND h.parent_qn = ? ORDER BY h.qn`,
		dimID, nextLevel, qn)
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

	if len(nodes) == 0 {
		elemRows, err := r.db().Query(`SELECT DISTINCT e.qn, e.kind FROM element_node en
			JOIN elements e ON e.qn = en.element_qn AND e.kind = en.element_kind
			WHERE en.dimension_id = ? AND en.node_qn = ? AND en.level = ? ORDER BY e.qn`, dimID, qn, nextLevel)
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
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func (r *Repository) pkgChildren(dimID int, qn string) ([]Node, error) {
	tree := r.pkgTree(dimID)
	nsLevel := r.LevelOfRole(dimID, "namespace")
	typeLevel := r.LevelOfRole(dimID, "type")

	kids := tree.children[qn]
	nodes := make([]Node, 0, len(kids))
	for _, k := range kids {
		n := Node{ID: k, Label: lastSeg(k), Kind: "PACKAGE", Size: tree.typeCount[k], Children: len(tree.children[k])}
		nodes = append(nodes, n)
	}

	typeRows, err := r.db().Query(`SELECT h.qn, h.kind,
			(SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = h.dimension_id AND en.node_qn = h.qn AND en.level = h.level + 1)
		FROM hierarchy_node h WHERE h.dimension_id = ? AND h.level = ? AND h.parent_qn = ? ORDER BY h.qn`,
		dimID, typeLevel, qn)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var n Node
		if err := typeRows.Scan(&n.ID, &n.Kind, &n.Size); err != nil {
			return nil, err
		}
		n.Label = lastSeg(n.ID)
		nodes = append(nodes, n)
	}
	typeRows.Close()
	_ = nsLevel
	return nodes, typeRows.Err()
}

func (r *Repository) NodeSize(dimID int, qn string, level int) int {
	nsLevel := r.LevelOfRole(dimID, "namespace")
	if level == nsLevel {
		return r.SubtreeTypeCount(dimID, qn)
	}
	var size int
	err := r.db().QueryRow(`SELECT
			(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = ? AND c.level = ? + 1 AND c.parent_qn = ?)
			+ (SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = ? AND en.node_qn = ? AND en.level = ? + 1)`,
		dimID, level, qn, dimID, qn, level).Scan(&size)
	if err != nil {
		return 0
	}
	return size
}

func (r *Repository) KindOf(dimID int, qn string) string {
	var kind string
	if err := r.db().QueryRow(`SELECT kind FROM hierarchy_node WHERE dimension_id = ? AND qn = ? LIMIT 1`, dimID, qn).Scan(&kind); err == nil {
		return kind
	}
	_ = r.db().QueryRow(`SELECT kind FROM elements WHERE qn = ? LIMIT 1`, qn).Scan(&kind)
	return kind
}

type TypeDetail struct {
	QN      string
	Kind    string
	Members []Member
}

type Member struct {
	QN        string
	Kind      string
	Name      string
	Signature string
}

func (r *Repository) DirectTypes(dimID int, pkg string) ([]TypeDetail, error) {
	typeLevel := r.LevelOfRole(dimID, "type")
	rows, err := r.db().Query(`SELECT h.qn, h.kind FROM hierarchy_node h
		WHERE h.dimension_id = ? AND h.level = ? AND h.parent_qn = ? ORDER BY h.qn`, dimID, typeLevel, pkg)
	if err != nil {
		return nil, err
	}
	var types []TypeDetail
	for rows.Next() {
		var t TypeDetail
		if err := rows.Scan(&t.QN, &t.Kind); err != nil {
			rows.Close()
			return nil, err
		}
		types = append(types, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range types {
		members, err := r.typeMembers(dimID, types[i].QN)
		if err != nil {
			return nil, err
		}
		types[i].Members = members
	}
	return types, nil
}

type TopItem struct {
	QN    string
	Label string
	Kind  string
	Value int
}

func (r *Repository) TopBy(dimID int, by string, limit int) ([]TopItem, error) {
	var rows *sql.Rows
	var err error
	switch by {
	case "fanout":
		rows, err = r.db().Query(`
SELECT f.qn, f.kind, f.fan_out FROM type_fan f
		WHERE f.dimension_id = ? ORDER BY f.fan_out DESC, f.qn LIMIT ?
`, dimID, limit)
	case "fanin":
		rows, err = r.db().Query(`
SELECT f.qn, f.kind, f.fan_in FROM type_fan f
		WHERE f.dimension_id = ? ORDER BY f.fan_in DESC, f.qn LIMIT ?
`, dimID, limit)
	default:
		rows, err = r.db().Query(`
SELECT h.qn, h.kind,
		(SELECT COUNT(*) FROM hierarchy_node c WHERE c.dimension_id = h.dimension_id AND c.level = h.level + 1 AND c.parent_qn = h.qn)
		+ (SELECT COUNT(*) FROM element_node en WHERE en.dimension_id = h.dimension_id AND en.node_qn = h.qn AND en.level = h.level + 1)
		FROM hierarchy_node h WHERE h.dimension_id = ? ORDER BY 3 DESC, h.qn LIMIT ?
`, dimID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TopItem, 0, limit)
	for rows.Next() {
		var t TopItem
		var kind sql.NullString
		if err := rows.Scan(&t.QN, &kind, &t.Value); err != nil {
			return nil, err
		}
		t.Kind = kind.String
		t.Label = lastSeg(t.QN)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) TypeToPackage(dimID int) map[string]string {
	out := map[string]string{}
	rows, err := r.db().Query(`SELECT qn, COALESCE(parent_qn,'') FROM hierarchy_node
		WHERE dimension_id = ? AND role = 'type'`, dimID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var qn, parent string
		if err := rows.Scan(&qn, &parent); err != nil {
			continue
		}
		out[qn] = parent
	}
	return out
}

func (r *Repository) PackageParent(dimID int, qn string) (string, bool) {
	var parent string
	err := r.db().QueryRow(`SELECT COALESCE(parent_qn,'') FROM hierarchy_node
		WHERE dimension_id = ? AND qn = ? AND role = 'namespace' LIMIT 1`, dimID, qn).Scan(&parent)
	if err != nil {
		return "", false
	}
	return parent, true
}
