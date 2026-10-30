package dto

// SccComp 强连通分量。
type SccComp struct {
	ID      int      `json:"id"`
	Members []string `json:"members"`
}

// SearchHit 搜索命中。
type SearchHit struct {
	QN        string `json:"qn"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	NodeQN    string `json:"node_qn,omitempty"`
	Level     int    `json:"level,omitempty"`
}

// TopItem 指标排序项。
type TopItem struct {
	QN    string `json:"qn"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Value int    `json:"value"`
}

// UnkHit 游离元素命中。
type UnkHit struct {
	QN   string `json:"qn"`
	Kind string `json:"kind"`
}

// MetaResp 数据可信度信息。
type MetaResp struct {
	ImportedAt    string         `json:"imported_at"`
	Stats         map[string]int `json:"stats"`
	IsolatedNodes int            `json:"isolated_nodes"`
	IsolatedList  []string       `json:"isolated_list"`
	UnknownKinds  int            `json:"unknown_kinds"`
	UnknownList   []UnkHit       `json:"unknown_list"`
	RelationTypes int            `json:"relation_types"`
}
