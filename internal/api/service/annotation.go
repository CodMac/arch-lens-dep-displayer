package service

import (
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// Annotations 列出该维度全部标注（收藏 + 备注）。
func (s *Service) Annotations(dimID int) ([]dto.Annotation, error) {
	items, err := s.repo.Annotations(dimID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Annotation, 0, len(items))
	for _, a := range items {
		out = append(out, dto.Annotation{
			DimensionID: a.DimensionID,
			QN:          a.QN,
			Starred:     a.Starred,
			Note:        a.Note,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
			Kind:        a.Kind,
		})
	}
	return out, nil
}

// UpsertAnnotation 收藏/备注 upsert（starred/note 为 nil 表示不修改）。
func (s *Service) UpsertAnnotation(dimID int, qn string, req dto.UpsertAnnotationReq) (dto.Annotation, error) {
	if err := s.repo.UpsertAnnotation(dimID, qn, req.Starred, req.Note); err != nil {
		return dto.Annotation{}, err
	}
	a, err := s.repo.GetAnnotation(dimID, qn)
	if err != nil {
		return dto.Annotation{}, err
	}
	return dto.Annotation{
		DimensionID: a.DimensionID,
		QN:          a.QN,
		Starred:     a.Starred,
		Note:        a.Note,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}, nil
}

// DeleteAnnotation 删除标注行（取消收藏）。
func (s *Service) DeleteAnnotation(dimID int, qn string) error {
	return s.repo.DeleteAnnotation(dimID, qn)
}
