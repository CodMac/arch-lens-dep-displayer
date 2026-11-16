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
