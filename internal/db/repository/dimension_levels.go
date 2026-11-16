package repository

func (r *Repository) LevelOfRole(dimID int, role string) int {
	var level int
	_ = r.db().QueryRow(`SELECT level FROM dimension_levels WHERE dimension_id = ? AND role = ?`, dimID, role).Scan(&level)
	return level
}

// DimensionLevels 返回某维度的层级定义（按 level 升序）。
func (r *Repository) DimensionLevels(dimID int) ([]Level, error) {
	rows, err := r.db().Query(`SELECT level, role, kinds FROM dimension_levels
		WHERE dimension_id = ? ORDER BY level`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Level
	for rows.Next() {
		var lv Level
		var kinds string
		if err := rows.Scan(&lv.Level, &lv.Role, &kinds); err != nil {
			return nil, err
		}
		lv.Kinds = SplitTypes(kinds)
		out = append(out, lv)
	}
	return out, rows.Err()
}

// ---- 写路径 ----

// ClearDimensionLevels 清空某维度的层级定义。
func (r *Repository) ClearDimensionLevels(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM dimension_levels WHERE dimension_id = ?`, dimID)
	return err
}

// InsertDimensionLevel 写入维度层级定义。
func (r *Repository) InsertDimensionLevel(dimID, level int, role, kinds string) error {
	_, err := r.db().Exec(`INSERT INTO dimension_levels (dimension_id, level, role, kinds) VALUES (?, ?, ?, ?)`,
		dimID, level, role, kinds)
	return err
}
