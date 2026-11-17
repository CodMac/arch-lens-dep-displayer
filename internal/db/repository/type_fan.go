package repository

// BuildTypeFan 生成类型级扇入扇出（P6）：与阈值无关的原始指标，孤立类型落全 0 行。
func (r *Repository) BuildTypeFan(dimID, typeLevel int) error {
	q := `INSERT INTO type_fan
			(dimension_id, qn, kind, fan_in, fan_out, fan_in_types, fan_out_types, fan_in_sites, fan_out_sites)
		WITH nodes AS (
			SELECT qn, kind FROM hierarchy_node WHERE dimension_id = ? AND level = ?
		),
		out_agg AS (
			SELECT source_qn AS qn, SUM(edge_count) AS w, COUNT(*) AS t, SUM(site_count) AS s
			FROM type_edge WHERE dimension_id = ? GROUP BY source_qn
		),
		in_agg AS (
			SELECT target_qn AS qn, SUM(edge_count) AS w, COUNT(*) AS t, SUM(site_count) AS s
			FROM type_edge WHERE dimension_id = ? GROUP BY target_qn
		)
		SELECT ?, n.qn, n.kind,
		       COALESCE(i.w,0), COALESCE(o.w,0),
		       COALESCE(i.t,0), COALESCE(o.t,0),
		       COALESCE(i.s,0), COALESCE(o.s,0)
		FROM nodes n
		LEFT JOIN out_agg o ON o.qn = n.qn
		LEFT JOIN in_agg  i ON i.qn = n.qn`
	_, err := r.db().Exec(q, dimID, typeLevel, dimID, dimID, dimID)
	return err
}

// ClearTypeFan 清空某维度的类型扇入扇出。
func (r *Repository) ClearTypeFan(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM type_fan WHERE dimension_id = ?`, dimID)
	return err
}

type FanRow struct {
	QN          string
	Kind        string
	FanIn       int
	FanOut      int
	FanInTypes  int
	FanOutTypes int
	FanInSites  int
	FanOutSites int
}

// IsolatedTypes 无跨类型边的类型（fan_in=0 且 fan_out=0），返回总数与明细。
func (r *Repository) IsolatedTypes(dimID, limit int) (int, []ElementRef, error) {
	const cond = `dimension_id = ? AND fan_in = 0 AND fan_out = 0`
	var total int
	if err := r.db().QueryRow(`SELECT COUNT(*) FROM type_fan WHERE `+cond, dimID).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := r.db().Query(`SELECT qn, COALESCE(kind,'') FROM type_fan WHERE `+cond+` ORDER BY qn LIMIT ?`, dimID, limit)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()
	var out []ElementRef
	for rows.Next() {
		var e ElementRef
		if err := rows.Scan(&e.QN, &e.Kind); err != nil {
			return total, out, err
		}
		out = append(out, e)
	}
	return total, out, rows.Err()
}

// TableCounts 返回预计算层各表在该维度下的行数（空表检查用）。
func (r *Repository) TableCounts(dimID int) (map[string]int, error) {
	tables := []string{"hierarchy_node", "element_node", "type_edge", "call_site", "type_fan",
		"violation_hit", "cycle_group", "pkg_rollup", "metrics"}
	out := map[string]int{}
	for _, t := range tables {
		var n int
		if err := r.db().QueryRow(`SELECT COUNT(*) FROM `+t+` WHERE dimension_id = ?`, dimID).Scan(&n); err != nil {
			return out, err
		}
		out[t] = n
	}
	return out, nil
}

func (r *Repository) TypeFans(dimID int) (map[string]FanRow, error) {
	rows, err := r.db().Query(`SELECT qn, COALESCE(kind,''), fan_in, fan_out, fan_in_types, fan_out_types,
		fan_in_sites, fan_out_sites FROM type_fan WHERE dimension_id = ?`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FanRow{}
	for rows.Next() {
		var f FanRow
		if err := rows.Scan(&f.QN, &f.Kind, &f.FanIn, &f.FanOut, &f.FanInTypes, &f.FanOutTypes,
			&f.FanInSites, &f.FanOutSites); err != nil {
			return nil, err
		}
		out[f.QN] = f
	}
	return out, rows.Err()
}
