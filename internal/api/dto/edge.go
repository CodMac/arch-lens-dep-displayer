package dto

// 边与调用点的出入参。

type TypeEdgeResp struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Count  int      `json:"count"`
	Types  []string `json:"types"`
}

type CallSiteResp struct {
	SourceQN   string `json:"source_qn"`
	SourceKind string `json:"source_kind"`
	TargetQN   string `json:"target_qn"`
	TargetKind string `json:"target_kind"`
	Type       string `json:"type"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Count      int    `json:"count"`
}
