package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/registry"
)

// ListSources 返回全部数据源（按创建时间排序）。
func (s *Service) ListSources() []dto.Source {
	list := s.mgr.List()
	out := make([]dto.Source, 0, len(list))
	for _, src := range list {
		out = append(out, mapSource(src))
	}
	return out
}

// CreateSource 注册新数据源并启动后台分析。返回 sid + 初始状态。
func (s *Service) CreateSource(req dto.CreateSourceReq) (string, dto.Source, error) {
	path := strings.TrimSpace(req.Path)
	lang := strings.TrimSpace(req.Language)
	if path == "" {
		return "", dto.Source{}, ErrPathRequired
	}
	if lang == "" {
		lang = "java" // 默认
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", dto.Source{}, ErrInvalidPath
	}
	sid, err := s.mgr.Create(path, lang)
	if err != nil {
		return "", dto.Source{}, err
	}
	src := s.mgr.Get(sid)
	if src == nil {
		return sid, dto.Source{}, ErrSourceNotFound
	}
	return sid, mapSource(*src), nil
}

// GetSource 返回某数据源状态（含步骤进度）。
func (s *Service) GetSource(sid string) (dto.Source, error) {
	src := s.mgr.Get(sid)
	if src == nil {
		return dto.Source{}, ErrSourceNotFound
	}
	return mapSource(*src), nil
}

// ActivateSource 切换当前激活数据源。
func (s *Service) ActivateSource(sid string) error {
	return s.mgr.Activate(sid)
}

// FsList 列出目录下一级内容；空 path 返回可浏览根（Windows 盘符）。
func (s *Service) FsList(dir string) (dto.FsListResult, error) {
	if dir == "" {
		return dto.FsListResult{Path: "", Items: listRoots()}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dto.FsListResult{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir() // 目录在前
		}
		return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
	})
	items := make([]dto.FsItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, dto.FsItem{Name: e.Name(), Path: filepath.Join(dir, e.Name()), IsDir: e.IsDir()})
	}
	return dto.FsListResult{Path: dir, Items: items}, nil
}

// mapSource registry.Source → dto.Source。
func mapSource(src registry.Source) dto.Source {
	return dto.Source{SID: src.SID, Path: src.Path, Language: src.Language, Status: src.Status, Active: src.Active, Steps: src.Steps, CreatedAt: src.CreatedAt}
}

// listRoots 返回可浏览根（Windows 盘符）。
func listRoots() []dto.FsItem {
	items := make([]dto.FsItem, 0)
	for _, d := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		p := string(d) + ":/"
		if _, err := os.ReadDir(p); err == nil {
			items = append(items, dto.FsItem{Name: p, Path: p, IsDir: true})
		}
	}
	return items
}
