package repository

import (
	"database/sql"
	"fmt"
)

// BuildMetrics 生成规模指标（P12，10 项）。
func (r *Repository) BuildMetrics(dimID, nsLevel, typeLevel int, typeKinds []string) error {
	q := fmt.Sprintf(`INSERT INTO metrics (dimension_id, kind, count)
		SELECT ?, 'element_total', COUNT(*) FROM elements
		UNION ALL SELECT ?, 'relation_total', COUNT(*) FROM relations
		UNION ALL SELECT ?, 'type_element_total', COUNT(*) FROM elements WHERE kind IN (%[1]s)
		UNION ALL SELECT ?, 'type_total', COUNT(*) FROM hierarchy_node WHERE dimension_id = ? AND level = %[2]d
		UNION ALL SELECT ?, 'package_total', COUNT(*) FROM hierarchy_node WHERE dimension_id = ? AND level = %[3]d
		UNION ALL SELECT ?, 'source_file_total', COUNT(DISTINCT path) FROM elements WHERE kind = 'FILE'
		UNION ALL SELECT ?, 'type_edge_total', COUNT(*) FROM type_edge WHERE dimension_id = ?
		UNION ALL SELECT ?, 'method_edge_total', COUNT(*) FROM method_edge WHERE dimension_id = ?
		UNION ALL SELECT ?, 'cycle_group_total', COUNT(*) FROM cycle_group WHERE dimension_id = ?
		UNION ALL SELECT ?, 'default_pkg_element_total', COUNT(*) FROM elements
			WHERE COALESCE(namespace_qn,'') IN ('', '(default)') AND kind NOT IN ('FILE','PACKAGE')`,
		sqlStringList(typeKinds), typeLevel, nsLevel)
	_, err := r.db().Exec(q,
		dimID,        // element_total
		dimID,        // relation_total
		dimID,        // type_element_total
		dimID, dimID, // type_total（维度 + 层级占位）
		dimID, dimID, // package_total（维度 + 层级占位）
		dimID,        // source_file_total
		dimID, dimID, // type_edge_total
		dimID, dimID, // method_edge_total
		dimID, dimID, // cycle_group_total
		dimID, // default_pkg_element_total
	)
	return err
}

// ClearMetrics 清空某维度的规模指标。
func (r *Repository) ClearMetrics(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM metrics WHERE dimension_id = ?`, dimID)
	return err
}

func (r *Repository) MetricsMap(dimID int) (map[string]int, error) {
	rows, err := r.db().Query(`SELECT kind, count FROM metrics WHERE dimension_id = ?`, dimID)
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

type UnkHit struct {
	QN   string
	Kind string
}

type Meta struct {
	ImportedAt    string
	Stats         map[string]int
	IsolatedNodes int
	IsolatedList  []string
	UnknownKinds  int
	UnknownList   []UnkHit
	RelationTypes int
}

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

	// 孤立节点：无出边且无入边的类型节点（type_fan 全 0 行）。
	isoQ := `
SELECT qn FROM type_fan
		WHERE dimension_id = ? AND fan_in = 0 AND fan_out = 0
`
	var isoCount int
	if err := r.db().QueryRow("SELECT COUNT(*) FROM ("+isoQ+")", dimID).Scan(&isoCount); err != nil {
		return nil, err
	}
	m.IsolatedNodes = isoCount
	isoRows, err := r.db().Query(isoQ+" ORDER BY qn LIMIT 50", dimID)
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
