package api

import (
	"encoding/json"
	"net/http"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/dto"
)

func (s *Server) listAnnotations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	items, err := s.svc.Annotations(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"items": items})
}

func (s *Server) upsertAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	if qn == "" {
		writeErr(w, 400, "missing qn")
		return
	}
	var req dto.UpsertAnnotationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid body: "+err.Error())
		return
	}
	if req.Starred == nil && req.Note == nil {
		writeErr(w, 400, "body 需提供 starred 或 note")
		return
	}
	a, err := s.svc.UpsertAnnotation(id, qn, req)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) deleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	if err := s.svc.DeleteAnnotation(id, qn); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "qn": qn})
}
