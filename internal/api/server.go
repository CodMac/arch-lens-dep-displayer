package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/service"
	"github.com/CodMac/arch-lens-dep-displayer/internal/registry"
)

// Server 聚合 handler 依赖：薄壳持有 service，业务逻辑全部下沉。
type Server struct {
	svc *service.Service
}

// New 创建 API server。
func New(mgr *registry.Registry) *Server {
	return &Server{svc: service.New(mgr)}
}

// ---- 响应工具 ----

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// pathQN 返回路径参数 qn（chi URLParam 已 URL 解码）。
func pathQN(r *http.Request) string {
	return chi.URLParam(r, "qn")
}
