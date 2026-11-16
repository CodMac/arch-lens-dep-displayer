package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// 源码定位与读取（读盘 + LRU + 路径安全校验）。

const (
	srcCacheMaxFiles = 64
	srcCacheMaxBytes = 32 << 20
)

type sourceCache struct {
	mu    sync.Mutex
	order []string
	data  map[string]string
	bytes int
}

var srcCache = &sourceCache{data: map[string]string{}}

func (c *sourceCache) get(path string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[path]
	return v, ok
}

func (c *sourceCache) put(path, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[path]; ok {
		return
	}
	c.data[path] = content
	c.order = append(c.order, path)
	c.bytes += len(content)
	for len(c.order) > srcCacheMaxFiles || c.bytes > srcCacheMaxBytes {
		old := c.order[0]
		c.order = c.order[1:]
		c.bytes -= len(c.data[old])
		delete(c.data, old)
	}
}

// SourceFile GET /source?qn=：按元素定位文件，读盘 + LRU，返回内容与元素树。
func (s *Service) SourceFile(dimID int, qn string) (dto.SourceFileResp, error) {
	path, ok := s.repo.ElementPath(qn)
	if !ok {
		// 包/类型节点：回退到其下任一元素所在文件。
		loc, err := s.repo.SourceOf(dimID, qn)
		if err != nil || loc.Path == "" {
			return dto.SourceFileResp{}, ErrDimNotFound
		}
		path = loc.Path
	}
	root := s.mgr.SourcePath()
	abs, err := safeJoin(root, path)
	if err != nil {
		return dto.SourceFileResp{}, err
	}

	content, ok := srcCache.get(abs)
	if !ok {
		b, err := os.ReadFile(abs)
		if err != nil {
			return dto.SourceFileResp{}, fmt.Errorf("read source %s: %w", path, err)
		}
		content = string(b)
		srcCache.put(abs, content)
	}

	metas, err := s.repo.SourceElements(dimID, path)
	if err != nil {
		return dto.SourceFileResp{}, err
	}
	return dto.SourceFileResp{
		Path:      path,
		Content:   content,
		LineCount: strings.Count(content, "\n") + 1,
		Elements:  buildElementTree(metas),
	}, nil
}

// safeJoin 拼接 root 与相对 path，并校验不越界。
func safeJoin(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("source root 未登记")
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	relCheck, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	return abs, nil
}

// buildElementTree 由 enclosing_qn 关系构建元素树（顶层为无 enclosing 的元素）。
func buildElementTree(metas []repository.ElementMeta) []dto.SourceElementResp {
	byQN := map[string]*dto.SourceElementResp{}
	var roots []*dto.SourceElementResp
	for _, m := range metas {
		node := &dto.SourceElementResp{
			QN: m.QN, Label: m.Name, Kind: m.Kind,
			IsSugar: m.IsSugar, IsImplDetail: m.IsImplDetail,
			StartLine: m.StartLine, EndLine: m.EndLine,
		}
		byQN[m.QN] = node
	}
	for _, m := range metas {
		node := byQN[m.QN]
		if parent, ok := byQN[m.EnclosingQN]; ok && parent != node {
			parent.Children = append(parent.Children, *node)
			continue
		}
		roots = append(roots, node)
	}
	out := make([]dto.SourceElementResp, 0, len(roots))
	for _, r := range roots {
		out = append(out, *r)
	}
	return out
}

func (s *Service) SourceOf(dimID int, qn string) (dto.SourceLoc, error) {
	loc, err := s.repo.SourceOf(dimID, qn)
	if err != nil {
		return dto.SourceLoc{}, ErrDimNotFound
	}
	return dto.SourceLoc{QN: loc.QN, Path: loc.Path, StartLine: loc.StartLine, Name: loc.Name}, nil
}
