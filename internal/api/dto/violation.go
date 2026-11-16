package dto

// 违规口径与结果的出入参。

type ViolationItem struct {
	QN        string `json:"qn"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Pkg       string `json:"pkg"`
	Metric    string `json:"metric"`
	Severity  string `json:"severity"`
	Value     int    `json:"value"`
	TypeCount int    `json:"typeCount"`
}

type CycleGroupResp struct {
	ID          int            `json:"id"`
	TypeMembers []string       `json:"typeMembers"`
	PkgMembers  []string       `json:"pkgMembers"`
	Hops        []CycleHopResp `json:"hops"`
}

// CycleHopResp 环组内边。
type CycleHopResp struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Types  []string `json:"types"`
	Sites  int      `json:"siteCount"`
}

// ViolationsResp 违规摘要 + 列表 + 环组。
type ViolationsResp struct {
	Config ViolationConfigResp `json:"config"`
	Fanin  []ViolationItem     `json:"fanin"`
	Fanout []ViolationItem     `json:"fanout"`
	Cycles []CycleGroupResp    `json:"cycles"`
}

// ViolationConfigResp 违规口径。
type ViolationConfigResp struct {
	FanWarn            int      `json:"fanWarn"`
	FanSevere          int      `json:"fanSevere"`
	CycleRelationTypes []string `json:"cycleRelationTypes"`
	MergeNested        bool     `json:"mergeNested"`
	DedupeSameSite     bool     `json:"dedupeSameSite"`
	UpdatedAt          string   `json:"updatedAt"`
}

// ViolationConfigReq 更新违规口径请求（nil 字段表示不修改）。
type ViolationConfigReq struct {
	FanWarn            *int     `json:"fanWarn"`
	FanSevere          *int     `json:"fanSevere"`
	CycleRelationTypes []string `json:"cycleRelationTypes"`
	MergeNested        *bool    `json:"mergeNested"`
	DedupeSameSite     *bool    `json:"dedupeSameSite"`
}
