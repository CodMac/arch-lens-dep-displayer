package api

import (
	"net/http"
	"strconv"
)

// 维度与全景：维度定义、指定层级顶层节点、首页概览。

func (s *Server) listDimensions(w http.ResponseWriter, r *http.Request) {
	dims, err := s.svc.Dimensions()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, dims)
}

func (s *Server) topLevel(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	level := 1
	if q := r.URL.Query().Get("level"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			level = v
		}
	}
	nodes, edges, err := s.svc.TopNodes(id, level)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": edges})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	id, ok := s.dimIDOr(w, r)
	if !ok {
		return
	}
	ov, err := s.svc.ProtoOverview(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ov)
}
