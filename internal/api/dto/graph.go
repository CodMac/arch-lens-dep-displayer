package dto

// 包树下钻通用结构：节点与边。

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
