package api

import (
	"net/http"
	"strconv"
	"strings"
)

// 蝴蝶图：双向 BFS 扇入扇出子图与全量扇入扇出指标。

func (s *Server) fanPayload(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := strings.TrimSpace(r.URL.Query().Get("qn"))
	if qn == "" {
		writeErr(w, 400, "missing qn")
		return
	}
	gran := r.URL.Query().Get("granularity")
	if gran == "" {
		gran = "type"
	}
	depth := 2
	if v := r.URL.Query().Get("depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10 {
			depth = n
		}
	}
	hide := r.URL.Query().Get("hideImplDetail") == "true"
	out, err := s.svc.Fan(id, qn, gran, depth, splitCSV(r.URL.Query().Get("types")), hide)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) fanStats(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.FanStats(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}
