package api

import (
	"net/http"
	"strings"
)

// 源码定位与读取（读盘 + 进程内 LRU + 路径安全校验）。

func (s *Server) source(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	loc, err := s.svc.SourceOf(id, qn)
	if err != nil {
		writeErr(w, 404, "source for "+qn+" not found")
		return
	}
	writeJSON(w, 200, loc)
}

func (s *Server) sourceFile(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := strings.TrimSpace(r.URL.Query().Get("qn"))
	if qn == "" {
		writeErr(w, 400, "missing qn")
		return
	}
	out, err := s.svc.SourceFile(id, qn)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, out)
}
