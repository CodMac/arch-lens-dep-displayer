package service

import (
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// 包树下钻与单节点邻域。

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

// FanOneWay 返回节点 qn 的单向扇入/扇出 BFS 子图（兼容旧 /fanin、/fanout 契约）。
func (s *Service) FanOneWay(dimID int, qn string, isIn bool, depth int) ([]dto.Node, []dto.Edge, error) {
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
