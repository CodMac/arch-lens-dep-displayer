package api

import (
	"net/http"
	"strconv"
)

// 包树下钻与单节点邻域：children / graph / fanin / fanout。

func (s *Server) children(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	nodes, err := s.svc.Children(id, qn)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": []interface{}{}})
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	nodes, edges, err := s.svc.Graph(id, qn, r.URL.Query().Get("types"))
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": edges})
}

func (s *Server) fanin(w http.ResponseWriter, r *http.Request) { s.fan(w, r, true) }

func (s *Server) fanout(w http.ResponseWriter, r *http.Request) { s.fan(w, r, false) }

func (s *Server) fan(w http.ResponseWriter, r *http.Request, isIn bool) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	qn := pathQN(r)
	depth := 2
	if q := r.URL.Query().Get("depth"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			depth = v
		}
	}
	nodes, edges, err := s.svc.FanOneWay(id, qn, isIn, depth)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": edges})
}
