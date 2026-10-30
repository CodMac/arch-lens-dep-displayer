package repository

import (
	"database/sql"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
)

// Annotation 标注记录（收藏 + 备注）。
type Annotation struct {
	DimensionID int
	QN          string
	Starred     bool
	Note        string
	CreatedAt   string
	UpdatedAt   string
	Kind        string
}

// Annotations 返回指定维度全部标注（kind 取自层级节点，无则空）。
func (r *Repository) Annotations(dimID int) ([]Annotation, error) {
	rows, err := r.db().Query(`
SELECT a.dimension_id, a.qn, a.starred, a.note, a.created_at, a.updated_at,
		COALESCE(h.kind, '')
	FROM annotations a
	LEFT JOIN hierarchy_node h ON h.dimension_id = a.dimension_id AND h.qn = a.qn
	WHERE a.dimension_id = ?
	ORDER BY a.qn
`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Annotation, 0)
	for rows.Next() {
		var a Annotation
		var ca, ua sql.NullString
		if err := rows.Scan(&a.DimensionID, &a.QN, &a.Starred, &a.Note, &ca, &ua, &a.Kind); err != nil {
			return nil, err
		}
		a.CreatedAt = ca.String
		a.UpdatedAt = ua.String
		items = append(items, a)
	}
	return items, rows.Err()
}

// GetAnnotation 回查单条标注（不存在返回错误）。
func (r *Repository) GetAnnotation(dimID int, qn string) (Annotation, error) {
	var a Annotation
	var ca, ua sql.NullString
	err := r.db().QueryRow(`SELECT dimension_id, qn, starred, note, created_at, updated_at FROM annotations WHERE dimension_id = ? AND qn = ?`, dimID, qn).Scan(&a.DimensionID, &a.QN, &a.Starred, &a.Note, &ca, &ua)
	if err != nil {
		return a, err
	}
	a.CreatedAt = ca.String
	a.UpdatedAt = ua.String
	return a, nil
}

// UpsertAnnotation 收藏/备注 upsert（部分更新，nil 保留原值）。
func (r *Repository) UpsertAnnotation(dimID int, qn string, starred *bool, note *string) error {
	return db.UpsertAnnotation(r.db(), dimID, qn, starred, note)
}

// DeleteAnnotation 删除标注行。
func (r *Repository) DeleteAnnotation(dimID int, qn string) error {
	return db.DeleteAnnotation(r.db(), dimID, qn)
}
