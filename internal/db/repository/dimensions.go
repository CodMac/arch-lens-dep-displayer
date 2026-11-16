package repository

import "strings"

type Dimension struct {
	ID          int
	Language    string
	QNSeparator string
	Levels      []Level
	// 依赖关系白名单（维度配置落库，供前端切片按钮使用）。
	RelationTypes []string
}

type Level struct {
	Level int
	Role  string
	Kinds []string
}

func (r *Repository) Dimensions() ([]Dimension, error) {
	rows, err := r.db().Query(`SELECT id, language, COALESCE(qn_separator,'.'),
		COALESCE(relation_types,'') FROM dimensions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dims := make([]Dimension, 0)
	for rows.Next() {
		var d Dimension
		var relTypes string
		if err := rows.Scan(&d.ID, &d.Language, &d.QNSeparator, &relTypes); err != nil {
			return nil, err
		}
		d.RelationTypes = SplitTypes(relTypes)
		dims = append(dims, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range dims {
		lrows, err := r.db().Query(`SELECT level, role, kinds FROM dimension_levels
			WHERE dimension_id = ? ORDER BY level`, dims[i].ID)
		if err != nil {
			return nil, err
		}
		for lrows.Next() {
			var lv Level
			var kinds string
			if err := lrows.Scan(&lv.Level, &lv.Role, &kinds); err != nil {
				lrows.Close()
				return nil, err
			}
			lv.Kinds = SplitTypes(kinds)
			dims[i].Levels = append(dims[i].Levels, lv)
		}
		lrows.Close()
	}
	return dims, nil
}

// ---- 写路径 ----

// DimensionID 返回指定语言的维度 id（不存在返回 0）。
func (r *Repository) DimensionID(language string) int {
	var id int
	_ = r.db().QueryRow(`SELECT id FROM dimensions WHERE language = ?`, language).Scan(&id)
	return id
}

// NextDimensionID 返回下一个可用的维度 id（MAX+1，DuckDB PRIMARY KEY 不自增）。
func (r *Repository) NextDimensionID() int {
	var maxID int
	_ = r.db().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM dimensions`).Scan(&maxID)
	return maxID + 1
}

// InsertDimension 写入维度行。
func (r *Repository) InsertDimension(id int, language, qnSeparator, methodKinds string) error {
	_, err := r.db().Exec(`INSERT INTO dimensions (id, language, qn_separator, method_kinds) VALUES (?, ?, ?, ?)`,
		id, language, qnSeparator, methodKinds)
	return err
}

// UpdateDimension 更新维度分隔符与方法级 kind 白名单。
func (r *Repository) UpdateDimension(id int, qnSeparator, methodKinds string) error {
	_, err := r.db().Exec(`UPDATE dimensions SET qn_separator = ?, method_kinds = ? WHERE id = ?`,
		qnSeparator, methodKinds, id)
	return err
}

// DimensionHeader 返回维度语言与 QN 分隔符。
func (r *Repository) DimensionHeader(dimID int) (language, qnSeparator string, err error) {
	err = r.db().QueryRow(`SELECT language, COALESCE(qn_separator,'.') FROM dimensions WHERE id = ?`, dimID).
		Scan(&language, &qnSeparator)
	return
}

// RegisterDimension 写入/更新维度定义与层级，返回维度 id（主表：dimensions）。
// DuckDB PRIMARY KEY 不自增，新建时用 MAX+1。
func (r *Repository) RegisterDimension(language, qnSeparator, methodKinds string, levels []Level,
	relationTypes, structuralTypes []string) (int, error) {
	id := r.DimensionID(language)
	if id == 0 {
		id = r.NextDimensionID()
		if err := r.InsertDimension(id, language, qnSeparator, methodKinds); err != nil {
			return 0, err
		}
	} else {
		if err := r.UpdateDimension(id, qnSeparator, methodKinds); err != nil {
			return 0, err
		}
		if err := r.ClearDimensionLevels(id); err != nil {
			return 0, err
		}
	}
	for _, lv := range levels {
		if err := r.InsertDimensionLevel(id, lv.Level, lv.Role, strings.Join(lv.Kinds, ",")); err != nil {
			return 0, err
		}
	}
	if err := r.UpdateDimensionRelationTypes(id,
		strings.Join(relationTypes, ","), strings.Join(structuralTypes, ",")); err != nil {
		return 0, err
	}
	return id, nil
}

// DimensionRelationTypes 返回维度落库的关系类型白名单（逗号分隔原串）。
func (r *Repository) DimensionRelationTypes(dimID int) (relationTypes, structuralTypes string, err error) {
	err = r.db().QueryRow(`SELECT COALESCE(relation_types,''), COALESCE(structural_relation_types,'')
		FROM dimensions WHERE id = ?`, dimID).Scan(&relationTypes, &structuralTypes)
	return
}

// UpdateDimensionRelationTypes 写入依赖关系白名单（逗号分隔）。
func (r *Repository) UpdateDimensionRelationTypes(id int, relationTypes, structuralTypes string) error {
	_, err := r.db().Exec(`UPDATE dimensions SET relation_types = ?, structural_relation_types = ? WHERE id = ?`,
		relationTypes, structuralTypes, id)
	return err
}
