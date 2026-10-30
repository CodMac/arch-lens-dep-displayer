package repository

import (
	"database/sql"
)

// SccComp 强连通分量。
type SccComp struct {
	ID      int
	Members []string
}

// SearchHit 搜索命中。
type SearchHit struct {
	QN        string
	Kind      string
	Name      string
	Path      string
	StartLine int
	NodeQN    string
	Level     int
}

// TopItem 指标排序项。
type TopItem struct {
	QN    string
	Label string
	Kind  string
	Value int
}

// UnkHit 游离元素命中。
type UnkHit struct {
	QN   string
	Kind string
}

// Meta 数据可信度信息。
type Meta struct {
	ImportedAt    string
	Stats         map[string]int
	IsolatedNodes int
	IsolatedList  []string
	UnknownKinds  int
	UnknownList   []UnkHit
	RelationTypes int
}

// SCC 返回全部强连通分量。
func (r *Repository) SCC(dimID int) ([]SccComp, error) {
	rows, err := r.db().Query(`SELECT component_id, member_qn FROM scc WHERE dimension_id = ? ORDER BY component_id`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	compMap := map[int]*SccComp{}
	var order []int
	for rows.Next() {
		var cid int
		var m string
		if err := rows.Scan(&cid, &m); err != nil {
			return nil, err
		}
		if _, ok := compMap[cid]; !ok {
			compMap[cid] = &SccComp{ID: cid}
			order = append(order, cid)
		}
		compMap[cid].Members = append(compMap[cid].Members, m)
	}
	out := make([]SccComp, 0, len(order))
	for _, cid := range order {
		out = append(out, *compMap[cid])
	}
	return out, rows.Err()
}

// Metrics 返回维度规模指标 map。
func (r *Repository) Metrics(dimID int) (map[string]int, error) {
	rows, err := r.db().Query(`SELECT kind, count FROM metrics WHERE dimension_id = ? ORDER BY kind`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var c int
		if err := rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		out[k] = c
	}
	return out, rows.Err()
}

// Search 按 QN/名称/路径模糊搜索元素（维度去重归属 + 层级归属）。
func (r *Repository) Search(dimID int, q string, limit int) ([]SearchHit, error) {
	like := "%" + q + "%"
	rows, err := r.db().Query(`
SELECT e.qn, e.kind, e.name, e.path, e.start_line,
		COALESCE(en.node_qn, ''), COALESCE(en.level, 0)
		FROM (SELECT qn, MIN(kind) AS kind, MIN(name) AS name, MIN(path) AS path, MIN(start_line) AS start_line
		      FROM elements WHERE qn ILIKE ? OR name ILIKE ? OR path ILIKE ?
		      GROUP BY qn ORDER BY qn LIMIT ?) e
		LEFT JOIN element_node en ON en.element_qn = e.qn AND en.dimension_id = ?
		ORDER BY e.qn
`, like, like, like, limit, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.QN, &h.Kind, &h.Name, &h.Path, &h.StartLine, &h.NodeQN, &h.Level); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// TopBy 按指标排序返回节点列表（fanout/fanin/size）。
func (r *Repository) TopBy(dimID int, by string, limit int) ([]TopItem, error) {
	var rows *sql.Rows
	var err error
	switch by {
	case "fanout":
		rows, err = r.db().Query(`
SELECT f.qn, h.kind, f.fan_out FROM fan f
		LEFT JOIN hierarchy_node h ON h.dimension_id = f.dimension_id AND h.qn = f.qn
		WHERE f.dimension_id = ? ORDER BY f.fan_out DESC, f.qn LIMIT ?
`, dimID, limit)
	case "fanin":
		rows, err = r.db().Query(`
SELECT f.qn, h.kind, f.fan_in FROM fan f
		LEFT JOIN hierarchy_node h ON h.dimension_id = f.dimension_id AND h.qn = f.qn
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

// MetaInfo 返回数据可信度信息：导入时间、统计、孤立节点、未知类型、关系类型数。
func (r *Repository) MetaInfo(dimID int) (*Meta, error) {
	m := &Meta{Stats: map[string]int{}}
	// 导入时间（dimensions.imported_at）。
	var importedAt sql.NullString
	if err := r.db().QueryRow("SELECT imported_at FROM dimensions WHERE id = ?", dimID).Scan(&importedAt); err != nil {
		return nil, err
	}
	m.ImportedAt = importedAt.String

	// 元素/关系统计（metrics 表）。
	mrows, err := r.db().Query("SELECT kind, count FROM metrics WHERE dimension_id = ?", dimID)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var k string
		var c int
		if err := mrows.Scan(&k, &c); err != nil {
			mrows.Close()
			return nil, err
		}
		m.Stats[k] = c
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	// 孤立节点：无出边且无入边的类级节点（level=2）。
	isoQ := `
SELECT h.qn FROM hierarchy_node h
		WHERE h.dimension_id = ? AND h.level = 2
		AND NOT EXISTS (SELECT 1 FROM level_edges e WHERE e.dimension_id = h.dimension_id AND e.level = 2 AND (e.source_qn = h.qn OR e.target_qn = h.qn))
`
	var isoCount int
	if err := r.db().QueryRow("SELECT COUNT(*) FROM ("+isoQ+")", dimID).Scan(&isoCount); err != nil {
		return nil, err
	}
	m.IsolatedNodes = isoCount
	isoRows, err := r.db().Query(isoQ+" ORDER BY h.qn LIMIT 50", dimID)
	if err != nil {
		return nil, err
	}
	for isoRows.Next() {
		var qn string
		if err := isoRows.Scan(&qn); err != nil {
			isoRows.Close()
			return nil, err
		}
		m.IsolatedList = append(m.IsolatedList, qn)
	}
	isoRows.Close()

	// 未知/游离元素（未纳入维度层级体系）。
	unkQ := `
SELECT e.qn, e.kind FROM elements e WHERE e.qn <> '' AND NOT EXISTS (SELECT 1 FROM element_node en WHERE en.element_qn = e.qn AND en.dimension_id = ?)
`
	var unkCount int
	if err := r.db().QueryRow("SELECT COUNT(*) FROM ("+unkQ+")", dimID).Scan(&unkCount); err != nil {
		return nil, err
	}
	m.UnknownKinds = unkCount
	unkRows, err := r.db().Query(unkQ+" ORDER BY qn LIMIT 50", dimID)
	if err != nil {
		return nil, err
	}
	for unkRows.Next() {
		var u UnkHit
		if err := unkRows.Scan(&u.QN, &u.Kind); err != nil {
			unkRows.Close()
			return nil, err
		}
		m.UnknownList = append(m.UnknownList, u)
	}
	unkRows.Close()

	// 关系类型分布（可信度参考）。
	if err := r.db().QueryRow("SELECT COUNT(DISTINCT type) FROM relations").Scan(&m.RelationTypes); err != nil {
		return nil, err
	}
	return m, nil
}
