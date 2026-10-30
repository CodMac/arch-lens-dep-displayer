package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
	"github.com/CodMac/arch-lens-dep-displayer/internal/api/service"
)

// dimID 解析路径维度 id。
func (s *Server) dimID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		return 0, false
	}
	return id, true
}

// ---- 数据源 - 薄壳 ----

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

func (s *Server) fsList(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	res, err := s.svc.FsList(dir)
	if err != nil {
		writeErr(w, 400, "无法读取目录: "+err.Error())
		return
	}
	writeJSON(w, 200, res)
}
