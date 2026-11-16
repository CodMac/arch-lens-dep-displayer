package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/service"
	"github.com/go-chi/chi/v5"
)

// 数据源管理：唯一触发 pipeline 的入口（POST /sources）。

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"items": s.svc.ListSources()})
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateSourceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid body: "+err.Error())
		return
	}
	sid, src, err := s.svc.CreateSource(req)
	if err != nil {
		status := 500
		if service.KnownErr(err) {
			status = 400
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"sid": sid, "source": src})
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")
	src, err := s.svc.GetSource(sid)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, src)
}

func (s *Server) activateSource(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")
	if err := s.svc.ActivateSource(sid); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "sid": sid})
}

// reanalyzeSource 清理旧数据并重新分析（POST /sources/{sid}/reanalyze）。
func (s *Server) reanalyzeSource(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")
	src, err := s.svc.ReanalyzeSource(sid)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"sid": sid, "source": src})
}

func (s *Server) fsList(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	res, err := s.svc.FsList(dir)
	if err != nil {
		writeErr(w, 400, "无法读取目录: "+err.Error())
		return
	}
	writeJSON(w, 200, res)
}
