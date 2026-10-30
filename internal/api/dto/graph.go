// Package dto 定义 API 出入参 schema（与前端契约对齐）。
package dto

// Node 图节点。Size=直接子节点数（规模）。
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Size  int    `json:"size"`
}

// Edge 图边。Type 为主类型（兼容旧前端），Types 为完整类型集。
type Edge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Type   string   `json:"type"`
	Types  []string `json:"types"`
	Count  int      `json:"count"`
}

// Level 维度的一个层级定义。
type Level struct {
	Level    int      `json:"level"`
	Kind     string   `json:"kind"`
	ParentOf []string `json:"parent_of"`
}

// Dimension 维度响应。
type Dimension struct {
	ID       int     `json:"id"`
	Language string  `json:"language"`
	Name     string  `json:"name"`
	Levels   []Level `json:"levels"`
}

// SourceLoc 源码定位响应（elements.path/start_line）。
type SourceLoc struct {
	QN        string `json:"qn"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	Name      string `json:"name"`
}
