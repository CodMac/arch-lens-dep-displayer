package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// 边与调用点。

func (s *Service) TypeEdges(dimID int, types []string) ([]dto.TypeEdgeResp, error) {
	edges, err := s.repo.TypeEdges(dimID, types)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TypeEdgeResp, 0, len(edges))
	for _, e := range edges {
		out = append(out, dto.TypeEdgeResp{Source: e.Source, Target: e.Target, Count: e.Count, Types: e.Types})
	}
	return out, nil
}

// MethodEdges 返回全量方法级边（method_edge VIEW，方法级 Blast Radius 数据源）。
func (s *Service) MethodEdges(dimID int) ([]dto.TypeEdgeResp, error) {
	edges, err := s.repo.MethodEdges(dimID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TypeEdgeResp, 0, len(edges))
	for _, e := range edges {
		out = append(out, dto.TypeEdgeResp{Source: e.Source, Target: e.Target, Count: e.Count, Types: e.Types})
	}
	return out, nil
}

func (s *Service) Calls(dimID int, source, target string) ([]dto.CallSiteResp, error) {
	sites, err := s.repo.CallSites(dimID, source, target)
	if err != nil {
		return nil, err
	}
	return mapCallSites(sites), nil
}

// MethodCalls 返回方法对的调用点明细。
func (s *Service) MethodCalls(dimID int, source, target string) ([]dto.CallSiteResp, error) {
	sites, err := s.repo.MethodCallSites(dimID, source, target)
	if err != nil {
		return nil, err
	}
	return mapCallSites(sites), nil
}

func mapCallSites(sites []repository.CallSite) []dto.CallSiteResp {
	out := make([]dto.CallSiteResp, 0, len(sites))
	for _, c := range sites {
		out = append(out, dto.CallSiteResp{
			SourceQN: c.SourceQN, SourceKind: c.SourceKind,
			TargetQN: c.TargetQN, TargetKind: c.TargetKind,
			Type: c.Type, Path: c.Path, Line: c.Line, Count: c.Count,
		})
	}
	return out
}
