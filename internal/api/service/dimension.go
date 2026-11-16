package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// 维度与全景：维度定义、指定层级顶层节点、首页概览。

func (s *Service) Dimensions() ([]dto.Dimension, error) {
	dims, err := s.repo.Dimensions()
	if err != nil {
		return nil, err
	}
	out := make([]dto.Dimension, 0, len(dims))
	for _, d := range dims {
		lv := make([]dto.Level, 0, len(d.Levels))
		for _, l := range d.Levels {
			lv = append(lv, dto.Level{Level: l.Level, Role: l.Role, Kinds: l.Kinds})
		}
		out = append(out, dto.Dimension{
			ID: d.ID, Language: d.Language, QNSeparator: d.QNSeparator, Levels: lv,
			RelationTypes: d.RelationTypes,
		})
	}
	return out, nil
}

// TopNodes 返回指定层级顶层节点 + 该层聚合边。
func (s *Service) TopNodes(dimID, level int) ([]dto.Node, []dto.Edge, error) {
	nodes, err := s.repo.TopNodes(dimID, level)
	if err != nil {
		return nil, nil, err
	}
	edges, err := s.repo.LevelEdges(dimID, level)
	if err != nil {
		return nil, nil, err
	}
	return mapNodes(nodes), mapEdges(edges), nil
}

func (s *Service) ProtoOverview(dimID int) (dto.ProtoOverview, error) {
	nodes, err := s.repo.OverviewNodes(dimID)
	if err != nil {
		return dto.ProtoOverview{}, err
	}
	edges, err := s.repo.LevelEdges(dimID, s.repo.LevelOfRole(dimID, "namespace"))
	if err != nil {
		return dto.ProtoOverview{}, err
	}
	tree, err := s.repo.PkgTreeStructure(dimID)
	if err != nil {
		return dto.ProtoOverview{}, err
	}
	metrics, err := s.repo.MetricsMap(dimID)
	if err != nil {
		return dto.ProtoOverview{}, err
	}
	groups, pkgs, faninPkgs, err := s.repo.ViolationSummary(dimID)
	if err != nil {
		return dto.ProtoOverview{}, err
	}

	out := dto.ProtoOverview{
		Metrics: dto.OverviewMetrics{
			PkgCount:    metrics["package_total"],
			TypeCount:   metrics["type_total"],
			RelCount:    metrics["relation_total"],
			CycleGroups: groups,
		},
		Violations: dto.ViolationSummary{CycleGroups: groups, CyclePkgs: pkgs, HighFaninPkgs: faninPkgs},
		Edges:      mapEdges(edges),
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, dto.OverviewNode{
			ID: n.ID, Label: n.Label, Kind: n.Kind, Size: n.Size,
			TypeCount: n.Size, Children: n.Children, Depth: n.Depth,
		})
	}
	for _, t := range tree {
		out.Tree = append(out.Tree, dto.PkgTreeNode{QN: t.QN, Parent: t.Parent, Depth: t.Depth})
	}
	return out, nil
}
