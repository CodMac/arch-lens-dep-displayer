package repository

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
			JOIN element_node en ON en.element_qn = e.qn AND en.dimension_id = ?
			WHERE en.node_qn = ? AND e.path <> '' ORDER BY e.start_line LIMIT 1`
		err2 := r.db().QueryRow(q2, dimID, qn).Scan(&loc.Path, &loc.Name, &loc.StartLine)
		if err2 != nil {
			return loc, err2
		}
	}
	loc.QN = qn
	return loc, nil
}
