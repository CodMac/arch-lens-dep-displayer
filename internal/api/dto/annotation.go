package dto

// Annotation 标注/收藏响应。
type Annotation struct {
	DimensionID int    `json:"dimension_id"`
	QN          string `json:"qn"`
	Starred     bool   `json:"starred"`
	Note        string `json:"note"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// UpsertAnnotationReq 收藏/备注 upsert 请求（nil 字段保留原值）。
type UpsertAnnotationReq struct {
	Starred *bool   `json:"starred"`
	Note    *string `json:"note"`
}
