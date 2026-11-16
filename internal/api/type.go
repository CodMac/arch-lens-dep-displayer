package api

import (
	"net/http"
	"strings"
)

// 类型与叶子包：包直接类型+成员、叶子包一次性负载、元素 kind。

func (s *Server) types(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.Types(id, r.URL.Query().Get("pkg"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// leaf GET /leaf?pkg=
func (s *Server) leaf(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.Leaf(id, r.URL.Query().Get("pkg"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) kind(w http.ResponseWriter, r *http.Request) {
	qn := strings.TrimSpace(r.URL.Query().Get("qn"))
	if qn == "" {
		writeErr(w, 400, "missing qn")
		return
	}
	out, err := s.svc.Kind(qn)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"qn": qn, "kind": out})
}
