package api

import (
	"net/http"
)

// 边与调用点：类型级/方法级边、类型对/方法对调用点。

func (s *Server) typeEdges(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.TypeEdges(id, splitCSV(r.URL.Query().Get("types")))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// methodEdges GET /method-edges
func (s *Server) methodEdges(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.MethodEdges(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) calls(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.Calls(id, r.URL.Query().Get("source"), r.URL.Query().Get("target"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// methodCalls GET /method-calls?source=&target=
func (s *Server) methodCalls(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	out, err := s.svc.MethodCalls(id, r.URL.Query().Get("source"), r.URL.Query().Get("target"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}
