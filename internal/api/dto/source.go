package dto

import "github.com/CodMac/arch-lens-dep-displayer/internal/pipeline"

// Source 数据源响应（含步骤进度）。
type Source struct {
	SID       string                `json:"sid"`
	Path      string                `json:"path"`
	Language  string                `json:"language"`
	Status    string                `json:"status"`
	Active    bool                  `json:"active"`
	Steps     []pipeline.StepResult `json:"steps,omitempty"`
	CreatedAt string                `json:"created_at"`
}

// CreateSourceReq 创建数据源请求。
type CreateSourceReq struct {
	Path     string `json:"path"`
	Language string `json:"language"`
}

// FsItem 目录项。
type FsItem struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// FsListResult 目录浏览响应。
type FsListResult struct {
	Path  string   `json:"path"`
	Items []FsItem `json:"items"`
}
