package api

import (
	"encoding/json"
	"net/http"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

// 违规口径与结果。PUT /violations/config 会重算派生表（唯一请求期写）。

func (s *Server) violations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.Violations(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) getViolationConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.ViolationConfig(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// putViolationConfig PUT /violations/config（落库 → 重算派生表 → 返回新摘要）
func (s *Server) putViolationConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	var req dto.ViolationConfigReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid body: "+err.Error())
		return
	}
	out, err := s.svc.UpdateViolationConfig(id, req)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}
