package dto

// 蝴蝶图的出入参。

type FanNode struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Side         string `json:"side"`
	Depth        int    `json:"depth"`
	HostType     string `json:"hostType,omitempty"`
	IsSugar      bool   `json:"isSugar"`
	IsImplDetail bool   `json:"isImplDetail"`
	CrossPkg     bool   `json:"crossPkg,omitempty"`
	FanIn        int    `json:"fanIn"`
	FanOut       int    `json:"fanOut"`
}

// FanEdge 蝴蝶图边。
type FanEdge struct {
	Source   string   `json:"source"`
	Target   string   `json:"target"`
	Type     string   `json:"type"`
	Types    []string `json:"types"`
	Count    int      `json:"count"`
	CrossPkg bool     `json:"crossPkg,omitempty"`
}

// FanStats 蝴蝶图统计（对齐前端 fanGraph.FanStats）。
type FanStats struct {
	UpCount    int `json:"upCount"`
	DownCount  int `json:"downCount"`
	CrossPkg   int `json:"crossPkg"`
	DirectUp   int `json:"directUp"`
	DirectDown int `json:"directDown"`
	FanIn      int `json:"fanIn"`
	FanOut     int `json:"fanOut"`
}

// FanNeighbor 直接邻居（节点详情用）。
type FanNeighbor struct {
	ID    string   `json:"id"`
	Count int      `json:"count"`
	Types []string `json:"types"`
}

// FanNodeStat 单节点扇入扇出明细（基于完整图，不受 depth/types 影响）。
type FanNodeStat struct {
	ID           string        `json:"id"`
	FanIn        int           `json:"fanIn"`
	FanOut       int           `json:"fanOut"`
	InNeighbors  []FanNeighbor `json:"inNeighbors"`
	OutNeighbors []FanNeighbor `json:"outNeighbors"`
}

// FanPayload 蝴蝶图负载。
type FanPayload struct {
	Target        string                 `json:"target"`
	Granularity   string                 `json:"granularity"`
	Nodes         []FanNode              `json:"nodes"`
	Edges         []FanEdge              `json:"edges"`
	Stats         FanStats               `json:"stats"`
	RelationTypes []string               `json:"relationTypes"`
	NodeStats     map[string]FanNodeStat `json:"nodeStats"`
}

type FanStatResp struct {
	QN          string `json:"qn"`
	Pkg         string `json:"pkg"`
	FanIn       int    `json:"fanIn"`
	FanOut      int    `json:"fanOut"`
	FanInTypes  int    `json:"fanInTypes"`
	FanOutTypes int    `json:"fanOutTypes"`
	FanInSites  int    `json:"fanInSites"`
	FanOutSites int    `json:"fanOutSites"`
}
