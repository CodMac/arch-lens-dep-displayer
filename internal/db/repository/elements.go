package repository

// ElementRow 元素表写入行（ingest 由 analyzer 模型转换而来）。
type ElementRow struct {
	QN           string
	Kind         string
	Name         string
	Path         string
	StartLine    int
	EndLine      int
	Signature    string
	Doc          string
	Comment      string
	NamespaceQN  string
	EnclosingQN  string
	TypeQN       string
	IsSource     bool
	IsSugar      bool
	IsImplDetail bool
	Extra        interface{} // 空串转 NULL（DuckDB JSON 列不接受空串）
}

// InsertElements 批量写入元素（主键 (qn, kind)，调用方负责去重）。
func (r *Repository) InsertElements(rows []ElementRow) error {
	data := make([][]interface{}, 0, len(rows))
	for _, e := range rows {
		data = append(data, []interface{}{
			e.QN, e.Kind, e.Name, e.Path, e.StartLine, e.EndLine,
			e.Signature, e.Doc, e.Comment,
			e.NamespaceQN, e.EnclosingQN, e.TypeQN,
			e.IsSource, e.IsSugar, e.IsImplDetail, e.Extra,
		})
	}
	const cols = `qn, kind, name, path, start_line, end_line, signature, doc, comment,
		namespace_qn, enclosing_qn, type_qn, is_source, is_sugar, is_impl_detail, extra`
	return r.insertBatches(`INSERT INTO elements (`+cols+`)`, 16, data)
}

// HierarchyElement 建层级树所需的元素字段快照。
type HierarchyElement struct {
	QN          string
	Kind        string
	Name        string
	Path        string
	StartLine   int
	EndLine     int
	NamespaceQN string
	EnclosingQN string
	TypeQN      string
}

// HierarchyElements 读取全部元素（建层级树用，跳过空 QN）。
func (r *Repository) HierarchyElements() ([]HierarchyElement, error) {
	rows, err := r.db().Query(`SELECT qn, kind, COALESCE(name,''), COALESCE(path,''),
			COALESCE(start_line,0), COALESCE(end_line,0),
			COALESCE(namespace_qn,''), COALESCE(enclosing_qn,''), COALESCE(type_qn,'')
		FROM elements`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HierarchyElement
	for rows.Next() {
		var e HierarchyElement
		if err := rows.Scan(&e.QN, &e.Kind, &e.Name, &e.Path, &e.StartLine, &e.EndLine,
			&e.NamespaceQN, &e.EnclosingQN, &e.TypeQN); err != nil {
			return nil, err
		}
		if e.QN == "" {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ElementNamespaces 返回 qn → namespace_qn（环组跨包判定用）。
func (r *Repository) ElementNamespaces() (map[string]string, error) {
	rows, err := r.db().Query(`SELECT qn, COALESCE(namespace_qn,'') FROM elements`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var qn, ns string
		if err := rows.Scan(&qn, &ns); err != nil {
			return nil, err
		}
		out[qn] = ns
	}
	return out, rows.Err()
}

func (r *Repository) ElementKind(qn string) (string, bool) {
	var kind string
	err := r.db().QueryRow(`SELECT kind FROM elements WHERE qn = ? LIMIT 1`, qn).Scan(&kind)
	if err != nil {
		return "", false
	}
	return kind, true
}

type ElementMeta struct {
	QN           string
	Kind         string
	Name         string
	Path         string
	StartLine    int
	EndLine      int
	EnclosingQN  string
	IsSugar      bool
	IsImplDetail bool
}

func (r *Repository) SourceElements(dimID int, path string) ([]ElementMeta, error) {
	rows, err := r.db().Query(`SELECT qn, kind, COALESCE(name,''), COALESCE(path,''),
			COALESCE(start_line,0), COALESCE(end_line,0), COALESCE(enclosing_qn,''),
			COALESCE(is_sugar,false), COALESCE(is_impl_detail,false)
		FROM elements WHERE path = ? ORDER BY start_line, end_line DESC`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ElementMeta
	for rows.Next() {
		var e ElementMeta
		if err := rows.Scan(&e.QN, &e.Kind, &e.Name, &e.Path, &e.StartLine, &e.EndLine,
			&e.EnclosingQN, &e.IsSugar, &e.IsImplDetail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ElementPath(qn string) (string, bool) {
	var path string
	err := r.db().QueryRow(`SELECT COALESCE(path,'') FROM elements WHERE qn = ? AND COALESCE(path,'') <> '' LIMIT 1`, qn).Scan(&path)
	if err != nil || path == "" {
		return "", false
	}
	return path, true
}

func (r *Repository) ElementFlags(qn string) (isSugar, isImplDetail bool, ok bool) {
	err := r.db().QueryRow(`SELECT COALESCE(is_sugar,false), COALESCE(is_impl_detail,false)
		FROM elements WHERE qn = ? LIMIT 1`, qn).Scan(&isSugar, &isImplDetail)
	if err != nil {
		return false, false, false
	}
	return isSugar, isImplDetail, true
}

func (r *Repository) HostTypeOf(qn string) string {
	var t string
	_ = r.db().QueryRow(`SELECT COALESCE(type_qn,'') FROM elements WHERE qn = ? LIMIT 1`, qn).Scan(&t)
	return t
}

type SearchHit struct {
	QN        string
	Kind      string
	Name      string
	Path      string
	StartLine int
	NodeQN    string
	Level     int
}

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

// SourceLoc 元素源码定位。
type SourceLoc struct {
	QN        string
	Path      string
	StartLine int
	Name      string
}

// SourceOf 返回元素 qn 的源文件路径与起始行（elements.path/start_line）。
// 优先查 elements（元素定义位置）；包节点无 path 时回退其下任一元素 path。
func (r *Repository) SourceOf(dimID int, qn string) (SourceLoc, error) {
	var loc SourceLoc
	q1 := `SELECT path, name, start_line FROM elements WHERE qn = ? AND path <> '' ORDER BY start_line LIMIT 1`
	err := r.db().QueryRow(q1, qn).Scan(&loc.Path, &loc.Name, &loc.StartLine)
	if err != nil {
		// 包节点：取其下任一元素 path 作为定位锚点。
		q2 := `SELECT e.path, e.name, e.start_line FROM elements e
			JOIN element_node en ON en.element_qn = e.qn AND en.element_kind = e.kind AND en.dimension_id = ?
			WHERE en.node_qn = ? AND e.path <> '' ORDER BY e.start_line LIMIT 1`
		err2 := r.db().QueryRow(q2, dimID, qn).Scan(&loc.Path, &loc.Name, &loc.StartLine)
		if err2 != nil {
			return loc, err2
		}
	}
	loc.QN = qn
	return loc, nil
}
