package repository

import (
	"database/sql"
)

type ViolationConfig struct {
	FanWarn            int
	FanSevere          int
	CycleRelationTypes []string
	MergeNested        bool
	DedupeSameSite     bool
	UpdatedAt          string
}

func (r *Repository) GetViolationConfig(dimID int) (ViolationConfig, error) {
	var c ViolationConfig
	var typesStr string
	var updated sql.NullString
	err := r.db().QueryRow(`SELECT fan_warn, fan_severe,
			COALESCE(CAST(cycle_relation_types AS VARCHAR),''), merge_nested, dedupe_same_site,
			CAST(updated_at AS VARCHAR)
		FROM violation_config WHERE dimension_id = ?`, dimID).
		Scan(&c.FanWarn, &c.FanSevere, &typesStr, &c.MergeNested, &c.DedupeSameSite, &updated)
	if err != nil {
		return c, err
	}
	c.CycleRelationTypes = SplitArrayString(typesStr)
	c.UpdatedAt = updated.String
	return c, nil
}

// EnsureViolationConfig 首次写入默认口径（已存在则保留用户设置）。
func (r *Repository) EnsureViolationConfig(dimID int, cycleTypes []string) error {
	var n int
	if err := r.db().QueryRow(`SELECT COUNT(*) FROM violation_config WHERE dimension_id = ?`, dimID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	q := `INSERT INTO violation_config
			(dimension_id, fan_warn, fan_severe, cycle_relation_types, merge_nested, dedupe_same_site)
		VALUES (?, 50, 150, ` + arrayLiteral(cycleTypes) + `, TRUE, TRUE)`
	_, err := r.db().Exec(q, dimID)
	return err
}

// CycleRelationTypes 返回环计算使用的关系类型（配置为空时回退 fallback）。
func (r *Repository) CycleRelationTypes(dimID int, fallback []string) []string {
	var raw string
	if err := r.db().QueryRow(`SELECT COALESCE(CAST(cycle_relation_types AS VARCHAR),'')
		FROM violation_config WHERE dimension_id = ?`, dimID).Scan(&raw); err != nil {
		return fallback
	}
	if out := SplitArrayString(raw); len(out) > 0 {
		return out
	}
	return fallback
}

func (r *Repository) SaveViolationConfig(dimID int, req ViolationConfigPatch) error {
	q := `UPDATE violation_config SET
			fan_warn = COALESCE(?, fan_warn),
			fan_severe = COALESCE(?, fan_severe),
			merge_nested = COALESCE(?, merge_nested),
			dedupe_same_site = COALESCE(?, dedupe_same_site),
			updated_at = now()
		WHERE dimension_id = ?`
	if _, err := r.db().Exec(q, req.FanWarn, req.FanSevere, req.MergeNested, req.DedupeSameSite, dimID); err != nil {
		return err
	}
	if len(req.CycleRelationTypes) > 0 {
		arr := "["
		for i, t := range req.CycleRelationTypes {
			if i > 0 {
				arr += ","
			}
			arr += "'" + t + "'"
		}
		arr += "]"
		if _, err := r.db().Exec(`UPDATE violation_config SET cycle_relation_types = `+arr+` WHERE dimension_id = ?`, dimID); err != nil {
			return err
		}
	}
	return nil
}

type ViolationConfigPatch struct {
	FanWarn            *int
	FanSevere          *int
	CycleRelationTypes []string
	MergeNested        *bool
	DedupeSameSite     *bool
}
