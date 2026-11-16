package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline"
)

// Create 注册一个新的数据源（源码目录）并立即启动后台分析 pipeline。
// 若 sid 已存在（相同路径），复用已有源，不重复创建。返回 sid。
// 分析为异步执行：调用方通过 Get()/PollStatus() 轮询进度。
func (r *Registry) Create(path, language string) (string, error) {
	sid := SID(path)

	r.mu.RLock()
	_, ok := r.sources[sid]
	r.mu.RUnlock()
	if ok {
		return sid, nil // 已存在：直接返回
	}

	ws := r.Workspace(sid)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}

	pl := pipeline.New(path, language, ws)
	src := &Source{
		SID:       sid,
		Path:      path,
		Language:  language,
		Status:    string(StatusAnalyzing),
		CreatedAt: timeNow(),
		pl:        pl,
	}

	r.mu.Lock()
	r.sources[sid] = src
	r.mu.Unlock()

	go r.runAnalyze(sid, pl)
	return sid, nil
}

// Reanalyze 清理该源的旧产物（output/、lens.duckdb、meta.json）后重新执行完整分析链。
// 旧数据不可恢复；若该源正被激活，先关闭缓存的 DuckDB 连接。
func (r *Registry) Reanalyze(sid string) error {
	r.mu.RLock()
	src, ok := r.sources[sid]
	var path, lang string
	if ok {
		path, lang = src.Path, src.Language
	}
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("source %s not found", sid)
	}

	// 关闭缓存连接（该源可能正是激活源），避免删库文件时被占用。
	r.Refresh()

	ws := r.Workspace(sid)
	if err := clearWorkspace(ws); err != nil {
		return fmt.Errorf("clear workspace: %w", err)
	}

	pl := pipeline.New(path, lang, ws)
	r.mu.Lock()
	src.pl = pl
	src.Status = string(StatusAnalyzing)
	src.Steps = pl.Results()
	r.mu.Unlock()

	go r.runAnalyze(sid, pl)
	return nil
}

// clearWorkspace 删除工作区内的分析产物（保留工作目录本身）。
func clearWorkspace(ws string) error {
	if err := os.RemoveAll(filepath.Join(ws, "output")); err != nil {
		return err
	}
	for _, name := range []string{"lens.duckdb", "lens.duckdb.wal", "meta.json"} {
		if err := os.Remove(filepath.Join(ws, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// runAnalyze 后台执行 pipeline（串行 6 步），结束后更新源状态。
func (r *Registry) runAnalyze(sid string, pl *pipeline.Pipeline) {
	err := pl.Run(context.Background())

	r.mu.Lock()
	src := r.sources[sid]
	if src == nil {
		r.mu.Unlock()
		return
	}
	src.Steps = pl.Results()
	if err != nil {
		src.Status = string(StatusFailed)
	} else {
		src.Status = string(StatusReady)
	}
	r.mu.Unlock()
}

// PollStatus 返回某源的最新步骤状态快照（供前端轮询）。
func (r *Registry) PollStatus(sid string) (Source, bool) {
	s := r.Get(sid)
	if s == nil {
		return Source{}, false
	}
	return *s, true
}
