package repository

// ElementNodeRow 元素挂载写入行。
type ElementNodeRow struct {
	ElementQN   string
	ElementKind string
	DimensionID int
	NodeQN      string
	Level       int
}

// InsertElementNodes 批量写入元素挂载。
func (r *Repository) InsertElementNodes(rows []ElementNodeRow) error {
	data := make([][]interface{}, 0, len(rows))
	for _, n := range rows {
		data = append(data, []interface{}{n.ElementQN, n.ElementKind, n.DimensionID, n.NodeQN, n.Level})
	}
	return r.insertBatches(`INSERT INTO element_node (element_qn, element_kind, dimension_id, node_qn, level)`, 5, data)
}

func (r *Repository) typeMembers(dimID int, typeQN string) ([]Member, error) {
	rows, err := r.db().Query(`SELECT e.qn, e.kind, COALESCE(e.name,''), COALESCE(e.signature,'')
		FROM element_node en
		JOIN elements e ON e.qn = en.element_qn AND e.kind = en.element_kind
		WHERE en.dimension_id = ? AND en.node_qn = ? AND en.element_qn <> ?
		ORDER BY e.qn`, dimID, typeQN, typeQN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.QN, &m.Kind, &m.Name, &m.Signature); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
