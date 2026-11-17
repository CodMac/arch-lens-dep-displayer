package repository

// RelationRow 关系表写入行（ingest 由 analyzer 模型转换而来）。
type RelationRow struct {
	SourceQN   string
	SourceKind string
	TargetQN   string
	TargetKind string
	Type       string
	Path       string
	StartLine  int
	EndLine    int
	RawText    interface{} // 空串转 NULL
}

// InsertRelations 批量写入关系。
func (r *Repository) InsertRelations(rows []RelationRow) error {
	data := make([][]interface{}, 0, len(rows))
	for _, rel := range rows {
		data = append(data, []interface{}{
			rel.SourceQN, rel.SourceKind, rel.TargetQN, rel.TargetKind,
			rel.Type, rel.Path, rel.StartLine, rel.EndLine, rel.RawText,
		})
	}
	const cols = `source_qn, source_kind, target_qn, target_kind, type, path, start_line, end_line, raw_text`
	return r.insertBatches(`INSERT INTO relations (`+cols+`)`, 9, data)
}

// CountDanglingRelations 统计端点 JOIN elements 落空的关系数（只计数告警，不补全）。
func (r *Repository) CountDanglingRelations() (int, error) {
	q := `SELECT COUNT(*) FROM relations r
		WHERE NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
		   OR NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.target_qn AND e.kind = r.target_kind)`
	var n int
	err := r.db().QueryRow(q).Scan(&n)
	return n, err
}

// ContainParents 返回 CONTAIN 关系的 child → parent 映射（命名空间树优先来源）。
func (r *Repository) ContainParents() (map[string]string, error) {
	rows, err := r.db().Query(`SELECT source_qn, target_qn FROM relations WHERE type = 'CONTAIN'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var s, t string
		if err := rows.Scan(&s, &t); err != nil {
			return nil, err
		}
		out[t] = s
	}
	return out, rows.Err()
}

// DanglingRel 端点 JOIN elements 落空的关系明细。
type DanglingRel struct {
	Type       string
	Path       string
	Line       int
	SourceQN   string
	SourceKind string
	TargetQN   string
	TargetKind string
	Reason     string // source_not_found | target_not_found | both_not_found
}

// DanglingRelations 返回端点在 elements 中不存在的关系统计与明细（明细 LIMIT limit）。
func (r *Repository) DanglingRelations(limit int) (int, []DanglingRel, error) {
	const cond = `NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
		   OR NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.target_qn AND e.kind = r.target_kind)`
	var total int
	if err := r.db().QueryRow(`SELECT COUNT(*) FROM relations r WHERE ` + cond).Scan(&total); err != nil {
		return 0, nil, err
	}
	q := `SELECT COALESCE(r.type,''), COALESCE(r.path,''), COALESCE(r.start_line,0),
			r.source_qn, r.source_kind, r.target_qn, r.target_kind,
			CASE
				WHEN NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
				 AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.target_qn AND e.kind = r.target_kind) THEN 'both_not_found'
				WHEN NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind) THEN 'source_not_found'
				ELSE 'target_not_found'
			END
		FROM relations r WHERE ` + cond + ` LIMIT ?`
	rows, err := r.db().Query(q, limit)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()
	var out []DanglingRel
	for rows.Next() {
		var d DanglingRel
		if err := rows.Scan(&d.Type, &d.Path, &d.Line, &d.SourceQN, &d.SourceKind,
			&d.TargetQN, &d.TargetKind, &d.Reason); err != nil {
			return total, out, err
		}
		out = append(out, d)
	}
	return total, out, rows.Err()
}

// KindMismatch 端点 qn 存在但 kind 不一致的关系明细。
type KindMismatch struct {
	Type     string
	Path     string
	Line     int
	QN       string
	Expected string
	Actual   string
	End      string // source | target
}

// KindMismatchRelations 返回端点 qn 存在但 kind 不匹配的关系（明细 LIMIT limit）。
func (r *Repository) KindMismatchRelations(limit int) (int, []KindMismatch, error) {
	const cond = `(EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn) AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind))
		   OR (EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.target_qn) AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.target_qn AND e.kind = r.target_kind))`
	var total int
	if err := r.db().QueryRow(`SELECT COUNT(*) FROM relations r WHERE ` + cond).Scan(&total); err != nil {
		return 0, nil, err
	}
	q := `SELECT COALESCE(r.type,''), COALESCE(r.path,''), COALESCE(r.start_line,0),
			CASE WHEN EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn) AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
				THEN r.source_qn ELSE r.target_qn END,
			CASE WHEN EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn) AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
				THEN r.source_kind ELSE r.target_kind END,
			(SELECT e.kind FROM elements e WHERE e.qn = CASE WHEN EXISTS (SELECT 1 FROM elements e2 WHERE e2.qn = r.source_qn) AND NOT EXISTS (SELECT 1 FROM elements e2 WHERE e2.qn = r.source_qn AND e2.kind = r.source_kind) THEN r.source_qn ELSE r.target_qn END LIMIT 1),
			CASE WHEN EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn) AND NOT EXISTS (SELECT 1 FROM elements e WHERE e.qn = r.source_qn AND e.kind = r.source_kind)
				THEN 'source' ELSE 'target' END
		FROM relations r WHERE ` + cond + ` LIMIT ?`
	rows, err := r.db().Query(q, limit)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()
	var out []KindMismatch
	for rows.Next() {
		var m KindMismatch
		if err := rows.Scan(&m.Type, &m.Path, &m.Line, &m.QN, &m.Expected, &m.Actual, &m.End); err != nil {
			return total, out, err
		}
		out = append(out, m)
	}
	return total, out, rows.Err()
}

// RelationTypeHistogram 关系类型分布（含结构关系）。
func (r *Repository) RelationTypeHistogram() (map[string]int, error) {
	rows, err := r.db().Query(`SELECT COALESCE(type,''), COUNT(*) FROM relations GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, rows.Err()
}

func (r *Repository) RelationTypes(dimID int) ([]string, error) {
	rows, err := r.db().Query(`SELECT DISTINCT type FROM relations WHERE type <> 'CONTAIN' ORDER BY type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
