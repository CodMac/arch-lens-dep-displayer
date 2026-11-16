package repository

import "fmt"

// BuildCallSite 生成调用点明细（P5）：端点保留元素自身 QN + kind，无 Location 写空 path / line=0。
func (r *Repository) BuildCallSite(dimID int, f TypeEdgeFilter) error {
	q := fmt.Sprintf(`INSERT INTO call_site
			(dimension_id, source_qn, source_kind, target_qn, target_kind, type, path, line, call_count)
		SELECT ?, r.source_qn, r.source_kind, r.target_qn, r.target_kind, r.type,
		       COALESCE(r.path,''), COALESCE(r.start_line,0), COUNT(*)
		FROM relations r
		JOIN elements s ON s.qn = r.source_qn AND s.kind = r.source_kind
		JOIN elements t ON t.qn = r.target_qn AND t.kind = r.target_kind
		WHERE %s
		  AND %s
		  AND s.kind NOT IN ('FILE','PACKAGE')
		  AND t.kind NOT IN ('FILE','PACKAGE')
		GROUP BY 1,2,3,4,5,6,7,8`,
		notInFilter("r.type", f.StructuralTypes), inFilter("r.type", f.RelationTypes))
	_, err := r.db().Exec(q, dimID)
	return err
}

// ClearCallSite 清空某维度的调用点明细。
func (r *Repository) ClearCallSite(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM call_site WHERE dimension_id = ?`, dimID)
	return err
}

type CallSite struct {
	SourceQN   string
	SourceKind string
	TargetQN   string
	TargetKind string
	Type       string
	Path       string
	Line       int
	Count      int
}

func (r *Repository) CallSites(dimID int, source, target string) ([]CallSite, error) {
	q := `SELECT cs.source_qn, cs.source_kind, cs.target_qn, cs.target_kind, cs.type, cs.path, cs.line, cs.call_count
		FROM call_site cs
		JOIN elements s ON s.qn = cs.source_qn AND s.kind = cs.source_kind
		JOIN elements t ON t.qn = cs.target_qn AND t.kind = cs.target_kind
		WHERE cs.dimension_id = ? AND s.type_qn = ? AND t.type_qn = ?
		ORDER BY cs.path, cs.line`
	rows, err := r.db().Query(q, dimID, source, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CallSite
	for rows.Next() {
		var c CallSite
		if err := rows.Scan(&c.SourceQN, &c.SourceKind, &c.TargetQN, &c.TargetKind, &c.Type, &c.Path, &c.Line, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) MethodCallSites(dimID int, source, target string) ([]CallSite, error) {
	q := `SELECT source_qn, source_kind, target_qn, target_kind, type, path, line, call_count
		FROM call_site WHERE dimension_id = ? AND source_qn = ? AND target_qn = ?
		ORDER BY path, line`
	rows, err := r.db().Query(q, dimID, source, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CallSite
	for rows.Next() {
		var c CallSite
		if err := rows.Scan(&c.SourceQN, &c.SourceKind, &c.TargetQN, &c.TargetKind, &c.Type, &c.Path, &c.Line, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
