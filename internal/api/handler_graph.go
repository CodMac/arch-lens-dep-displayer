package api

import (
	"net/http"
	"strconv"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/service"
)

// dimIDOr 解析维度 id，失败写 400 并返回 false。
func (s *Server) dimIDOr(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := s.dimID(r)
	if !ok {
		writeErr(w, 400, "bad dimension id")
		return 0, false
	}
	return id, true
}

// errStatus 将 service 错误映射为 HTTP 状态码。
func errStatus(err error) int {
	if service.KnownErr(err) {
		return 404
	}
	return 500
}

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
	nodes, edges, err := s.svc.Overview(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": edges})
}

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

func (s *Server) fanin(w http.ResponseWriter, r *http.Request)  { s.fan(w, r, true) }
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
	nodes, edges, err := s.svc.Fan(id, qn, isIn, depth)
	if err != nil {
		writeErr(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"nodes": nodes, "edges": edges})
}
