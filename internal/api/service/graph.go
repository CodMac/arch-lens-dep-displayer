package service

import (
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// Dimensions 返回全部维度（含层级定义）。
func (s *Service) Dimensions() ([]dto.Dimension, error) {
	dims, err := s.repo.Dimensions()
	if err != nil {
		return nil, err
	}
	out := make([]dto.Dimension, 0, len(dims))
	for _, d := range dims {
		lv := make([]dto.Level, 0, len(d.Levels))
		for _, l := range d.Levels {
			lv = append(lv, dto.Level{Level: l.Level, Kind: l.Kind, ParentOf: l.ParentOf})
		}
		out = append(out, dto.Dimension{ID: d.ID, Language: d.Language, Name: d.Name, Levels: lv})
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

// Overview 首页全景：level=1 顶层包节点 + 包间聚合边。
func (s *Service) Overview(dimID int) ([]dto.Node, []dto.Edge, error) {
	nodes, err := s.repo.OverviewNodes(dimID)
	if err != nil {
		return nil, nil, err
	}
	edges, err := s.repo.LevelEdges(dimID, 1)
	if err != nil {
		return nil, nil, err
	}
	return mapNodes(nodes), mapEdges(edges), nil
}

// Children 返回节点 qn 的下一层子节点。
func (s *Service) Children(dimID int, qn string) ([]dto.Node, error) {
	level, ok := s.repo.NodeLevel(dimID, qn)
	if !ok {
		return nil, ErrDimNotFound
	}
	nodes, err := s.repo.ChildNodes(dimID, qn, level, level+1)
	if err != nil {
		return nil, err
	}
	return mapNodes(nodes), nil
}

// Graph 返回以 qn 为焦点、其邻接边为主的子图。
func (s *Service) Graph(dimID int, qn string, typesFilter string) ([]dto.Node, []dto.Edge, error) {
	level, ok := s.repo.NodeLevel(dimID, qn)
	if !ok {
		return nil, nil, ErrDimNotFound
	}
	var types []string
	if typesFilter != "" {
		for _, t := range strings.Split(typesFilter, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				types = append(types, t)
			}
		}
	}
	edges, err := s.repo.FocusEdges(dimID, level, qn, types)
	if err != nil {
		return nil, nil, err
	}
	nodes := map[string]dto.Node{
		qn: {ID: qn, Label: lastSeg(qn), Kind: s.repo.KindOf(dimID, qn), Size: s.repo.NodeSize(dimID, qn, level)},
	}
	for _, e := range edges {
		for _, other := range []string{e.Source, e.Target} {
			if _, ok := nodes[other]; !ok {
				nodes[other] = dto.Node{ID: other, Label: lastSeg(other)}
			}
		}
	}
	out := make([]dto.Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n)
	}
	return out, mapEdges(edges), nil
}

// Fan 返回节点 qn 的入/出扇 BFS 可达子图（level=2 类级边）。
func (s *Service) Fan(dimID int, qn string, isIn bool, depth int) ([]dto.Node, []dto.Edge, error) {
	level, ok := s.repo.NodeLevel(dimID, qn)
	if !ok {
		return nil, nil, ErrDimNotFound
	}
	nodes := map[string]dto.Node{
		qn: {ID: qn, Label: lastSeg(qn), Kind: s.repo.KindOf(dimID, qn), Size: s.repo.NodeSize(dimID, qn, level)},
	}
	var edges []dto.Edge
	visited := map[string]bool{qn: true}
	frontier := []string{qn}
	for d := 0; d < depth; d++ {
		var next []string
		for _, f := range frontier {
			fes, err := s.repo.FanEdges(dimID, level, f, isIn)
			if err != nil {
				return nil, nil, err
			}
			for _, e := range fes {
				edges = append(edges, mapEdge(e))
				other := e.Target
				if isIn {
					other = e.Source
				}
				if !visited[other] {
					visited[other] = true
					nodes[other] = dto.Node{ID: other, Label: lastSeg(other)}
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	out := make([]dto.Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n)
	}
	return out, edges, nil
}

// SourceOf 返回元素源码定位。
func (s *Service) SourceOf(dimID int, qn string) (dto.SourceLoc, error) {
	loc, err := s.repo.SourceOf(dimID, qn)
	if err != nil {
		return dto.SourceLoc{}, ErrDimNotFound
	}
	return dto.SourceLoc{QN: loc.QN, Path: loc.Path, StartLine: loc.StartLine, Name: loc.Name}, nil
}
