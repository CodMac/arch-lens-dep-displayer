package dto

// 维度与全景的出入参：维度定义、层级、包树、首页概览。

type Dimension struct {
	ID            int      `json:"id"`
	Language      string   `json:"language"`
	QNSeparator   string   `json:"qn_separator"`
	Levels        []Level  `json:"levels"`
	RelationTypes []string `json:"relation_types"`
}

type Level struct {
	Level int      `json:"level"`
	Role  string   `json:"role"`
	Kinds []string `json:"kinds"`
}

type PkgTreeNode struct {
	QN     string `json:"qn"`
	Parent string `json:"parent"`
	Depth  int    `json:"depth"`
}

type OverviewMetrics struct {
	PkgCount    int `json:"pkgCount"`
	TypeCount   int `json:"typeCount"`
	RelCount    int `json:"relCount"`
	CycleGroups int `json:"cycleGroups"`
}

// ViolationSummary 违规摘要（仪表盘摘要行）。
type ViolationSummary struct {
	CycleGroups   int `json:"cycleGroups"`
	CyclePkgs     int `json:"cyclePkgs"`
	HighFaninPkgs int `json:"highFaninPkgs"`
}

// OverviewNode 全景节点（包气泡）。
type OverviewNode struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Size      int    `json:"size"`
	TypeCount int    `json:"typeCount"`
	Children  int    `json:"children"`
	Depth     int    `json:"depth"`
}

// ProtoOverview 全景负载（扩展版 /overview）。
type ProtoOverview struct {
	Metrics    OverviewMetrics  `json:"metrics"`
	Violations ViolationSummary `json:"violations"`
	Nodes      []OverviewNode   `json:"nodes"`
	Edges      []Edge           `json:"edges"`
	Tree       []PkgTreeNode    `json:"tree"`
}
