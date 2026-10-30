package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) scc(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	comps, err := s.svc.SCC(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, comps)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.Metrics(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, 400, "missing q")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	hits, err := s.svc.Search(id, q, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"query": q, "hits": hits})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	m, err := s.svc.Meta(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) topBy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "fanout"
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := s.svc.TopBy(id, by, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"by": by, "items": items})
}
