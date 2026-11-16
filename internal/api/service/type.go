package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// 类型与叶子包。

func (s *Service) Types(dimID int, pkg string) ([]dto.TypeResp, error) {
	types, err := s.repo.DirectTypes(dimID, pkg)
	if err != nil {
		return nil, err
	}
	return mapTypes(types), nil
}

// Leaf 返回叶子包负载：类型节点 + 类型间边 + 成员。
func (s *Service) Leaf(dimID int, pkg string) (dto.LeafResp, error) {
	types, err := s.repo.DirectTypes(dimID, pkg)
	if err != nil {
		return dto.LeafResp{}, err
	}
	all, err := s.repo.TypeEdges(dimID, nil)
	if err != nil {
		return dto.LeafResp{}, err
	}
	inPkg := map[string]bool{}
	for _, t := range types {
		inPkg[t.QN] = true
	}
	out := dto.LeafResp{Pkg: pkg, Members: map[string][]dto.MemberResp{}}
	for _, t := range types {
		out.Nodes = append(out.Nodes, dto.TypeResp{
			ID: t.QN, Label: lastSeg(t.QN), Kind: t.Kind, Members: mapMembers(t.Members),
		})
		out.Members[t.QN] = mapMembers(t.Members)
	}
	for _, e := range all {
		if inPkg[e.Source] && inPkg[e.Target] {
			out.Edges = append(out.Edges, dto.TypeEdgeResp{Source: e.Source, Target: e.Target, Count: e.Count, Types: e.Types})
		}
	}
	return out, nil
}

func (s *Service) Kind(qn string) (string, error) {
	kind, ok := s.repo.ElementKind(qn)
	if !ok {
		return "", ErrDimNotFound
	}
	return kind, nil
}

func mapTypes(types []repository.TypeDetail) []dto.TypeResp {
	out := make([]dto.TypeResp, 0, len(types))
	for _, t := range types {
		out = append(out, dto.TypeResp{ID: t.QN, Label: lastSeg(t.QN), Kind: t.Kind, Members: mapMembers(t.Members)})
	}
	return out
}

func mapMembers(ms []repository.Member) []dto.MemberResp {
	out := make([]dto.MemberResp, 0, len(ms))
	for _, m := range ms {
		out = append(out, dto.MemberResp{ID: m.QN, Label: lastSeg(m.QN), Kind: m.Kind, Signature: m.Signature})
	}
	return out
}
