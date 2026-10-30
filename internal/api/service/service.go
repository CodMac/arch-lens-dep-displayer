// Package service 提供 API 业务逻辑层：统一调度 registry + repository，
// 将查询结果映射为 dto（与前端契约对齐）。handler 只保留薄壳：解析参数 + 调 service + 写响应。
package service

import (
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/registry"
	"github.com/CodMac/arch-lens-dep-displayer/internal/repository"
)

// Service API 业务层，聚合执行依赖。
type Service struct {
	mgr  *registry.Registry
	repo *repository.Repository
}

// New 创建服务层。repo 通过 mgr.ActiveDB 获取当前激活源连接。
func New(mgr *registry.Registry) *Service {
	return &Service{mgr: mgr, repo: repository.New(mgr.ActiveDB)}
}

// ActiveSID 当前激活数据源 sid。
func (s *Service) ActiveSID() string { return s.mgr.ActiveSID() }

// mapNode repository.Node → dto.Node。
func mapNode(n repository.Node) dto.Node {
	return dto.Node{ID: n.ID, Label: n.Label, Kind: n.Kind, Size: n.Size}
}

// mapNodes repository.Node 列表 → dto.Node 列表。
func mapNodes(ns []repository.Node) []dto.Node {
	out := make([]dto.Node, 0, len(ns))
	for _, n := range ns {
		out = append(out, mapNode(n))
	}
	return out
}

// mapEdge repository.Edge → dto.Edge。
func mapEdge(e repository.Edge) dto.Edge {
	return dto.Edge{Source: e.Source, Target: e.Target, Type: e.Type, Types: e.Types, Count: e.Count}
}

// mapEdges repository.Edge 列表 → dto.Edge 列表。
func mapEdges(es []repository.Edge) []dto.Edge {
	out := make([]dto.Edge, 0, len(es))
	for _, e := range es {
		out = append(out, mapEdge(e))
	}
	return out
}

// lastSeg 返回 QN 最后一段（label 用）。
func lastSeg(qn string) string {
	if i := strings.LastIndex(qn, "."); i >= 0 {
		return qn[i+1:]
	}
	return qn
}
