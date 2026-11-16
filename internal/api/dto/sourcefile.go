package dto

// 源码定位与读取的出入参。

type SourceLoc struct {
	QN        string `json:"qn"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	Name      string `json:"name"`
}

type SourceElementResp struct {
	QN           string              `json:"qn"`
	Label        string              `json:"label"`
	Kind         string              `json:"kind"`
	IsSugar      bool                `json:"isSugar"`
	IsImplDetail bool                `json:"isImplDetail"`
	StartLine    int                 `json:"startLine"`
	EndLine      int                 `json:"endLine"`
	Children     []SourceElementResp `json:"children,omitempty"`
}

// SourceFileResp 源码文件响应。
type SourceFileResp struct {
	Path      string              `json:"path"`
	Content   string              `json:"content"`
	LineCount int                 `json:"lineCount"`
	Elements  []SourceElementResp `json:"elements"`
}
