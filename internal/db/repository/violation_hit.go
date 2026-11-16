package repository

import "fmt"

// BuildViolationHit 按当前 violation_config 阈值从 type_fan 命中（P7）。
// dedupe_same_site=true 时用 fan_*_sites 比较，否则用加权 fan_*。
func (r *Repository) BuildViolationHit(dimID int) error {
	var warn, severe int
	var dedupe bool
	if err := r.db().QueryRow(`SELECT fan_warn, fan_severe, dedupe_same_site FROM violation_config WHERE dimension_id = ?`, dimID).
		Scan(&warn, &severe, &dedupe); err != nil {
		return err
	}
	val := func(metric string) string {
		if dedupe {
			return "fan_" + metric + "_sites"
		}
		return "fan_" + metric
	}
	cnt := func(metric string) string { return "fan_" + metric + "_types" }

	q := fmt.Sprintf(`INSERT INTO violation_hit (dimension_id, qn, metric, severity, value, type_count)
		SELECT ?, qn, 'fanin', 'severe', %[1]s, %[2]s FROM type_fan
		 WHERE dimension_id = ? AND %[1]s >= ?
		UNION ALL
		SELECT ?, qn, 'fanin', 'warn', %[1]s, %[2]s FROM type_fan
		 WHERE dimension_id = ? AND %[1]s >= ? AND %[1]s < ?
		UNION ALL
		SELECT ?, qn, 'fanout', 'severe', %[3]s, %[4]s FROM type_fan
		 WHERE dimension_id = ? AND %[3]s >= ?
		UNION ALL
		SELECT ?, qn, 'fanout', 'warn', %[3]s, %[4]s FROM type_fan
		 WHERE dimension_id = ? AND %[3]s >= ? AND %[3]s < ?`,
		val("in"), cnt("in"), val("out"), cnt("out"))
	_, err := r.db().Exec(q, dimID, dimID, severe, dimID, dimID, warn, severe,
		dimID, dimID, severe, dimID, dimID, warn, severe)
	return err
}

// WarnHitQNs 返回某 metric 下 warn 档命中的类型 QN（包级上卷用）。
func (r *Repository) WarnHitQNs(dimID int, metric string) ([]string, error) {
	rows, err := r.db().Query(`SELECT qn FROM violation_hit
		WHERE dimension_id = ? AND metric = ? AND severity = 'warn'`, dimID, metric)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var qn string
		if err := rows.Scan(&qn); err != nil {
			return nil, err
		}
		out = append(out, qn)
	}
	return out, rows.Err()
}

// ClearViolationHit 清空某维度的违规命中。
func (r *Repository) ClearViolationHit(dimID int) error {
	_, err := r.db().Exec(`DELETE FROM violation_hit WHERE dimension_id = ?`, dimID)
	return err
}

func (r *Repository) ViolationSummary(dimID int) (groups, pkgs, faninPkgs int, err error) {
	if err = r.db().QueryRow(`SELECT COUNT(*) FROM cycle_group WHERE dimension_id = ?`, dimID).Scan(&groups); err != nil {
		return
	}
	if err = r.db().QueryRow(`SELECT COUNT(DISTINCT namespace_qn) FROM cycle_member WHERE dimension_id = ?`, dimID).Scan(&pkgs); err != nil {
		return
	}
	// 高扇入包：包内直接类型有 fanin 违规命中（warn + severe 都算，命中表按 qn 只留最高档）。
	q := `SELECT COUNT(DISTINCT h.parent_qn) FROM violation_hit v
		JOIN hierarchy_node h ON h.dimension_id = v.dimension_id AND h.qn = v.qn AND h.role = 'type'
		WHERE v.dimension_id = ? AND v.metric = 'fanin'`
	err = r.db().QueryRow(q, dimID).Scan(&faninPkgs)
	return
}

type ViolationHit struct {
	QN        string
	Metric    string
	Severity  string
	Value     int
	TypeCount int
}

func (r *Repository) ViolationHits(dimID int) ([]ViolationHit, error) {
	rows, err := r.db().Query(`SELECT v.qn, v.metric, v.severity, v.value, v.type_count
		FROM violation_hit v WHERE v.dimension_id = ? ORDER BY v.metric, v.value DESC`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ViolationHit
	for rows.Next() {
		var h ViolationHit
		if err := rows.Scan(&h.QN, &h.Metric, &h.Severity, &h.Value, &h.TypeCount); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
