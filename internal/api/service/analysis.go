package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// SCC 返回全部强连通分量。
func (s *Service) SCC(dimID int) ([]dto.SccComp, error) {
	comps, err := s.repo.SCC(dimID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SccComp, 0, len(comps))
	for _, c := range comps {
		out = append(out, dto.SccComp{ID: c.ID, Members: c.Members})
	}
	return out, nil
}

// Metrics 返回维度规模指标 map。
func (s *Service) Metrics(dimID int) (map[string]int, error) {
	return s.repo.Metrics(dimID)
}

// Search 按 QN/名称/路径模糊搜索元素。limit 已由 handler 校验。
func (s *Service) Search(dimID int, q string, limit int) ([]dto.SearchHit, error) {
	hits, err := s.repo.Search(dimID, q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, dto.SearchHit{QN: h.QN, Kind: h.Kind, Name: h.Name, Path: h.Path, StartLine: h.StartLine, NodeQN: h.NodeQN, Level: h.Level})
	}
	return out, nil
}

// Meta 返回数据可信度信息。
func (s *Service) Meta(dimID int) (dto.MetaResp, error) {
	m, err := s.repo.MetaInfo(dimID)
	if err != nil {
		return dto.MetaResp{}, err
	}
	unk := make([]dto.UnkHit, 0, len(m.UnknownList))
	for _, u := range m.UnknownList {
		unk = append(unk, dto.UnkHit{QN: u.QN, Kind: u.Kind})
	}
	return dto.MetaResp{
		ImportedAt:    m.ImportedAt,
		Stats:         m.Stats,
		IsolatedNodes: m.IsolatedNodes,
		IsolatedList:  m.IsolatedList,
		UnknownKinds:  m.UnknownKinds,
		UnknownList:   unk,
		RelationTypes: m.RelationTypes,
	}, nil
}

// TopBy 按指标（fanout/fanin/size）返回排序节点列表。
func (s *Service) TopBy(dimID int, by string, limit int) ([]dto.TopItem, error) {
	items, err := s.repo.TopBy(dimID, by, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TopItem, 0, len(items))
	for _, it := range items {
		out = append(out, dto.TopItem{QN: it.QN, Label: it.Label, Kind: it.Kind, Value: it.Value})
	}
	return out, nil
}
