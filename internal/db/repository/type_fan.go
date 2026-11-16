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
