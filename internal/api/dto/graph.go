// Package dto 定义 API 出入参 schema（与前端契约对齐）。
package dto

// Node 图节点。Size=规模（包=子树总类数，类=成员数）；Children=子包数（>0 表示可展开）；
// Depth=包树深度（顶层=0，仅包节点有意义）。
type Node struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Size     int    `json:"size"`
	Children int    `json:"children,omitempty"`
	Depth    int    `json:"depth,omitempty"`
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

// PkgTreeNode 包树结构项（overview 附带，供前端构建完整父链与包深度）。
type PkgTreeNode struct {
	QN     string `json:"qn"`
	Parent string `json:"parent"`
	Depth  int    `json:"depth"`
}

// Overview 响应：顶层包节点 + 包间边 + 完整包树结构。
type Overview struct {
	Nodes []Node        `json:"nodes"`
	Edges []Edge        `json:"edges"`
	Tree  []PkgTreeNode `json:"tree"`
}

// SourceLoc 源码定位响应（elements.path/start_line）。
type SourceLoc struct {
	QN        string `json:"qn"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	Name      string `json:"name"`
}
