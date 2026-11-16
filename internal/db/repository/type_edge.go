package repository

import "fmt"

// TypeEdgeFilter 类型级边/调用点的关系过滤条件（维度配置驱动）。
type TypeEdgeFilter struct {
	RelationTypes   []string // 依赖关系白名单；空=不限
	StructuralTypes []string // 结构关系黑名单
}

// BuildTypeEdge 生成类型级边（P3）：端点归一化到 type_qn、按 (source,target,cross_pkg) 聚合。
func (r *Repository) BuildTypeEdge(dimID int, f TypeEdgeFilter) error {
	q := fmt.Sprintf(`INSERT INTO type_edge (dimension_id, source_qn, target_qn, types, edge_count, site_count, cross_pkg)
		SELECT ?, s.type_qn, t.type_qn,
		       array_agg(DISTINCT r.type),
		       COUNT(*),
		       COUNT(DISTINCT CASE WHEN COALESCE(r.start_line,0) > 0
		                           THEN r.path || ':' || r.start_line
		                           ELSE 'NOLOC:' || r.source_qn END),
		       COALESCE(NULLIF(s.namespace_qn,''), '(default)') <> COALESCE(NULLIF(t.namespace_qn,''), '(default)')
		FROM relations r
		JOIN elements s ON s.qn = r.source_qn AND s.kind = r.source_kind
		JOIN elements t ON t.qn = r.target_qn AND t.kind = r.target_kind
		WHERE %s
		  AND %s
		  AND COALESCE(s.type_qn,'') <> '' AND COALESCE(t.type_qn,'') <> ''
		  AND s.type_qn <> t.type_qn
		  AND s.kind NOT IN ('FILE','PACKAGE')
		  AND t.kind NOT IN ('FILE','PACKAGE')
		GROUP BY 1,2,3,7`,
		notInFilter("r.type", f.StructuralTypes), inFilter("r.type", f.RelationTypes))
	_, err := r.db().Exec(q, dimID)
	return err
}

// ClearTypeEdge 清空某维度的类型级边。
func (r *Repository) ClearTypeEdge(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM type_edge WHERE dimension_id = ?`, dimID)
	return err
}

// TypeEdgeRec 类型级边记录（环计算用）。
type TypeEdgeRec struct {
	Source string
	Target string
	Types  []string
	Sites  int
}

// TypeEdgesWithTypes 读取类型级边（含关系类型数组与引用点）。
func (r *Repository) TypeEdgesWithTypes(dimID int) ([]TypeEdgeRec, error) {
	rows, err := r.db().Query(`SELECT source_qn, target_qn, CAST(types AS VARCHAR), site_count
		FROM type_edge WHERE dimension_id = ?`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TypeEdgeRec
	for rows.Next() {
		var e TypeEdgeRec
		var typesStr string
		if err := rows.Scan(&e.Source, &e.Target, &typesStr, &e.Sites); err != nil {
			return nil, err
		}
		e.Types = SplitArrayString(typesStr)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) LevelEdges(dimID, level int) ([]Edge, error) {
	nsLevel := r.LevelOfRole(dimID, "namespace")
	typeLevel := r.LevelOfRole(dimID, "type")
	if level == nsLevel {
		q := `SELECT hs.parent_qn, ht.parent_qn, SUM(e.edge_count),
				CAST(list_distinct(flatten(list(e.types))) AS VARCHAR) AS types
			FROM type_edge e
			JOIN hierarchy_node hs ON hs.dimension_id = e.dimension_id AND hs.qn = e.source_qn AND hs.level = ?
			JOIN hierarchy_node ht ON ht.dimension_id = e.dimension_id AND ht.qn = e.target_qn AND ht.level = ?
			WHERE e.dimension_id = ? AND hs.parent_qn <> ht.parent_qn
			GROUP BY 1,2 ORDER BY 1,2`
		return r.scanEdges(q, typeLevel, typeLevel, dimID)
	}
	_ = typeLevel
	q := `SELECT source_qn, target_qn, edge_count, CAST(types AS VARCHAR) FROM type_edge
		WHERE dimension_id = ? ORDER BY source_qn, target_qn`
	return r.scanEdges(q, dimID)
}

func (r *Repository) scanEdges(q string, args ...interface{}) ([]Edge, error) {
	rows, err := r.db().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edges []Edge
	for rows.Next() {
		var e Edge
		var typesStr string
		if err := rows.Scan(&e.Source, &e.Target, &e.Count, &typesStr); err != nil {
			return nil, err
		}
		e.Types = SplitArrayString(typesStr)
		if len(e.Types) > 0 {
			e.Type = e.Types[0]
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func (r *Repository) FocusEdges(dimID, level int, qn string, types []string) ([]Edge, error) {
	all, err := r.LevelEdges(dimID, level)
	if err != nil {
		return nil, err
	}
	var out []Edge
	for _, e := range all {
		if e.Source != qn && e.Target != qn {
			continue
		}
		if len(types) > 0 && !intersects(e.Types, types) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *Repository) FanEdges(dimID, level int, qn string, isIn bool) ([]Edge, error) {
	all, err := r.LevelEdges(dimID, level)
	if err != nil {
		return nil, err
	}
	var out []Edge
	for _, e := range all {
		if isIn && e.Target == qn {
			out = append(out, e)
		} else if !isIn && e.Source == qn {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *Repository) TypeEdges(dimID int, types []string) ([]Edge, error) {
	all, err := r.LevelEdges(dimID, r.LevelOfRole(dimID, "type"))
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return all, nil
	}
	out := make([]Edge, 0, len(all))
	for _, e := range all {
		if intersects(e.Types, types) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *Repository) MethodEdges(dimID int) ([]Edge, error) {
	q := `SELECT source_qn, target_qn, edge_count, CAST(types AS VARCHAR) FROM method_edge
		WHERE dimension_id = ? ORDER BY source_qn, target_qn`
	return r.scanEdges(q, dimID)
}
