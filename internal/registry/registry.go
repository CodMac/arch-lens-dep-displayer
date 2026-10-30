// Package registry 管理多个依赖分析数据源（源码目录 → 已分析结果）。
// 每个数据源有独立的 workspace（sources/{sid}/），内含 meta.json / output/ / lens.duckdb。
// 职责：数据源集合的注册与激活切换 + 惰性打开 DuckDB 连接。
package registry

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline"
)

// SourceStatus 数据源状态。
type SourceStatus string

const (
	StatusUnanalyzed SourceStatus = "unanalyzed" // 未分析（仅记录路径）
	StatusAnalyzing  SourceStatus = "analyzing"  // 分析中
	StatusReady      SourceStatus = "ready"      // 已就绪（可直接浏览）
	StatusFailed     SourceStatus = "failed"     // 分析失败
)

// Source 一个数据源（源码目录）的元信息。
type Source struct {
	SID       string                `json:"sid"`
	Path      string                `json:"path"`
	Language  string                `json:"language"`
	Status    string                `json:"status"`
	Active    bool                  `json:"active"`
	Steps     []pipeline.StepResult `json:"steps,omitempty"`
	CreatedAt string                `json:"created_at"`
	pl        *pipeline.Pipeline    `json:"-"` // 内部持有（不序列化）
}

// Registry 数据源注册表：管"有哪些源、当前选哪个、连接怎么开"。
type Registry struct {
	rootDir string // sources/ 根目录

	mu      sync.RWMutex
	sources map[string]*Source
	active  string // 当前激活 sid（空=无）

	// 惰性打开的 DuckDB 连接（当前激活源）。
	dbMu sync.Mutex
	db   *sql.DB
}

// New 创建注册表。rootDir 为 sources/ 根目录。
func New(rootDir string) (*Registry, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create sources dir: %w", err)
	}
	r := &Registry{
		rootDir: rootDir,
		sources: make(map[string]*Source),
	}
	if err := r.scanExisting(); err != nil {
		return nil, err
	}
	return r, nil
}

// scanExisting 扫描 rootDir 下已有的源目录（含 meta.json 的），恢复为已知源。
func (r *Registry) scanExisting() error {
	entries, err := os.ReadDir(r.rootDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sid := e.Name()
		metaPath := filepath.Join(r.rootDir, sid, "meta.json")
		b, err := os.ReadFile(metaPath)
		if err != nil {
			continue // 无 meta.json 的目录不算数据源
		}
		var mf pipeline.MetaFile
		if err := json.Unmarshal(b, &mf); err != nil {
			continue
		}
		status := string(StatusReady)
		if mf.Status == "failed" {
			status = string(StatusFailed)
		} else if mf.Status != "ready" {
			status = string(StatusAnalyzing)
		}
		r.sources[sid] = &Source{
			SID:       sid,
			Path:      mf.SourcePath,
			Language:  mf.Language,
			Status:    status,
			Steps:     mf.Steps,
			CreatedAt: mf.CreatedAt,
		}
	}
	return nil
}

// List 返回全部数据源（按创建时间排序），并标记当前激活。
func (r *Registry) List() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Source, 0, len(r.sources))
	for _, s := range r.sources {
		cp := *s
		cp.Active = s.SID == r.active
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// Get 返回指定 sid 的数据源快照；不存在返回 nil。
func (r *Registry) Get(sid string) *Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sources[sid]
	if !ok {
		return nil
	}
	cp := *s
	cp.Active = s.SID == r.active
	// 分析中的源：实时取 pipeline 步骤状态（供前端轮询）。
	if cp.pl != nil {
		cp.Steps = cp.pl.Results()
		cp.Status = statusFromPipeline(cp.pl.Status())
	}
	return &cp
}

// statusFromPipeline 将 pipeline 整体状态映射为源状态。
func statusFromPipeline(s string) string {
	switch s {
	case "ready":
		return string(StatusReady)
	case "failed":
		return string(StatusFailed)
	default:
		return string(StatusAnalyzing)
	}
}

// Activate 将 sid 设为当前激活源（切换后 API 的 ActiveDB 将指向该源）。
func (r *Registry) Activate(sid string) error {
	r.mu.RLock()
	_, ok := r.sources[sid]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("source %s not found", sid)
	}
	r.mu.Lock()
	r.active = sid
	r.mu.Unlock()
	// 激活源变化：关闭旧缓存连接，下次 ActiveDB 惰性重开。
	r.Refresh()
	return nil
}

// ActiveSID 返回当前激活源 sid（空=无激活）。
func (r *Registry) ActiveSID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// Workspace 返回某 sid 的工作目录。
func (r *Registry) Workspace(sid string) string {
	return filepath.Join(r.rootDir, sid)
}

// SourceOf 返回某 sid 对应的源码路径与语言。
func (r *Registry) SourceOf(sid string) (path, lang string) {
	s := r.Get(sid)
	if s == nil {
		return "", ""
	}
	return s.Path, s.Language
}

// SID 计算源码路径的稳定哈希 ID（sha1[:12]）。
func SID(path string) string {
	h := sha1.Sum([]byte(path))
	return hex.EncodeToString(h[:6])
}

// timeNow 返回 RFC3339 时间字符串。
func timeNow() string {
	return time.Now().Format(time.RFC3339)
}
