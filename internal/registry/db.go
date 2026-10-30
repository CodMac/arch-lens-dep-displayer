package registry

import (
	"database/sql"
	"path/filepath"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
)

// ActiveDB 返回当前激活源的 DuckDB 连接（惰性打开并缓存）。
// 无激活源时返回 nil。
func (r *Registry) ActiveDB() *sql.DB {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db != nil {
		return r.db
	}
	sid := r.ActiveSID()
	if sid == "" {
		return nil
	}
	dbPath := filepath.Join(r.Workspace(sid), "lens.duckdb")
	c, err := db.Open(dbPath)
	if err != nil {
		return nil
	}
	if err := db.Init(c); err != nil {
		db.Close(c)
		return nil
	}
	r.db = c
	return r.db
}

// Refresh 强制关闭当前缓存连接（供源切换后重建）。
func (r *Registry) Refresh() {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db != nil {
		db.Close(r.db)
		r.db = nil
	}
}

// Close 关闭全部资源（数据源连接）。
func (r *Registry) Close() {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db != nil {
		db.Close(r.db)
		r.db = nil
	}
}
