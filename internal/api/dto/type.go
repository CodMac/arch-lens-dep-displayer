package dto

// 类型与叶子包的出入参。

type MemberResp struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
}

// TypeResp 类型节点 + 成员（/types、/leaf）。
type TypeResp struct {
	ID        string       `json:"id"`
	Label     string       `json:"label"`
	Kind      string       `json:"kind"`
	TypeCount int          `json:"typeCount,omitempty"`
	Children  int          `json:"children,omitempty"`
	Members   []MemberResp `json:"members,omitempty"`
}

// LeafResp 叶子包负载：类型节点 + 类型间边 + 成员。
type LeafResp struct {
	Pkg     string                  `json:"pkg"`
	Nodes   []TypeResp              `json:"nodes"`
	Edges   []TypeEdgeResp          `json:"edges"`
	Members map[string][]MemberResp `json:"members"`
}
