package pipeline

import (
	"database/sql"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
	"github.com/CodMac/arch-lens-dep-displayer/internal/dimension"
)

// registerDimension 将维度配置写入 dimensions / dimension_levels 表，返回 id。
// DuckDB PRIMARY KEY 不自增，手动 MAX+1。
func registerDimension(c *sql.DB, dim *dimension.Dimension, name string) (int, error) {
	existing := db.DimensionID(c, name)
	if existing == 0 {
		existing = db.NextDimensionID(c)
		if err := db.InsertDimension(c, existing, name, dim.Language); err != nil {
			return 0, err
		}
	} else {
		if err := db.ClearDimensionLevels(c, existing); err != nil {
			return 0, err
		}
	}
	for _, lv := range dim.Hierarchy {
		po := ""
		for i, k := range lv.ParentOf {
			if i > 0 {
				po += ","
			}
			po += k
		}
		if err := db.InsertDimensionLevel(c, existing, lv.Level, lv.Kind, po); err != nil {
			return 0, err
		}
	}
	return existing, nil
}
