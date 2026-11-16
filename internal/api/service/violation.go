package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline/preprocess"
)

// 违规口径与结果。PUT /violations/config 是唯一请求期写派生表的接口。

func (s *Service) Violations(dimID int) (dto.ViolationsResp, error) {
	cfg, err := s.repo.GetViolationConfig(dimID)
	if err != nil {
		return dto.ViolationsResp{}, err
	}
	hits, err := s.repo.ViolationHits(dimID)
	if err != nil {
		return dto.ViolationsResp{}, err
	}
	groups, err := s.repo.CycleGroups(dimID)
	if err != nil {
		return dto.ViolationsResp{}, err
	}
	out := dto.ViolationsResp{
		Config: dto.ViolationConfigResp{
			FanWarn: cfg.FanWarn, FanSevere: cfg.FanSevere,
			CycleRelationTypes: cfg.CycleRelationTypes,
			MergeNested:        cfg.MergeNested, DedupeSameSite: cfg.DedupeSameSite,
			UpdatedAt: cfg.UpdatedAt,
		},
	}
	pkgOf := s.repo.TypeToPackage(dimID)
	for _, h := range hits {
		item := dto.ViolationItem{
			QN: h.QN, Label: lastSeg(h.QN), Kind: s.repo.KindOf(dimID, h.QN), Pkg: pkgOf[h.QN],
			Metric: h.Metric, Severity: h.Severity, Value: h.Value, TypeCount: h.TypeCount,
		}
		if h.Metric == "fanin" {
			out.Fanin = append(out.Fanin, item)
		} else {
			out.Fanout = append(out.Fanout, item)
		}
	}
	for _, g := range groups {
		cg := dto.CycleGroupResp{ID: g.ID, TypeMembers: g.Members, PkgMembers: g.Pkgs}
		for _, h := range g.Hops {
			cg.Hops = append(cg.Hops, dto.CycleHopResp{Source: h.Source, Target: h.Target, Types: h.Types, Sites: h.Sites})
		}
		out.Cycles = append(out.Cycles, cg)
	}
	return out, nil
}

func (s *Service) ViolationConfig(dimID int) (dto.ViolationConfigResp, error) {
	cfg, err := s.repo.GetViolationConfig(dimID)
	if err != nil {
		return dto.ViolationConfigResp{}, err
	}
	return dto.ViolationConfigResp{
		FanWarn: cfg.FanWarn, FanSevere: cfg.FanSevere,
		CycleRelationTypes: cfg.CycleRelationTypes,
		MergeNested:        cfg.MergeNested, DedupeSameSite: cfg.DedupeSameSite,
		UpdatedAt: cfg.UpdatedAt,
	}, nil
}

// UpdateViolationConfig 落库口径并触发派生表重算，返回新摘要。
func (s *Service) UpdateViolationConfig(dimID int, req dto.ViolationConfigReq) (dto.ViolationsResp, error) {
	patch := repository.ViolationConfigPatch{
		FanWarn: req.FanWarn, FanSevere: req.FanSevere,
		CycleRelationTypes: req.CycleRelationTypes,
		MergeNested:        req.MergeNested, DedupeSameSite: req.DedupeSameSite,
	}
	if err := s.repo.SaveViolationConfig(dimID, patch); err != nil {
		return dto.ViolationsResp{}, err
	}
	if err := preprocess.RebuildDerived(s.repo, dimID); err != nil {
		return dto.ViolationsResp{}, err
	}
	return s.Violations(dimID)
}
