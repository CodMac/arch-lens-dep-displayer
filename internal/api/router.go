// Package api 提供 REST 接口（chi 路由），对齐 docs/middleware.md §6。
// 分层：router（路由注册）→ handler（薄壳）→ service（业务逻辑）→ registry/repository。
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Routes 构建 chi 路由。
func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Get("/sources", s.listSources)
		r.Post("/sources", s.createSource)
		r.Route("/sources/{sid}", func(r chi.Router) {
			r.Get("/", s.getSource)
			r.Post("/activate", s.activateSource)
		})
		r.Get("/fs/list", s.fsList)
		r.With(s.requireDB).Route("/dimensions", func(r chi.Router) {
			r.Get("/", s.listDimensions)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/top", s.topLevel)
				r.Get("/overview", s.overview)
				r.Get("/nodes/{qn}/children", s.children)
				r.Get("/nodes/{qn}/graph", s.graph)
				r.Get("/nodes/{qn}/source", s.source)
				r.Get("/nodes/{qn}/fanin", s.fanin)
				r.Get("/nodes/{qn}/fanout", s.fanout)
				r.Get("/scc", s.scc)
				r.Get("/metrics", s.metrics)
				r.Get("/search", s.search)
				r.Get("/meta", s.meta)
				r.Get("/topby", s.topBy)
				r.Get("/annotations", s.listAnnotations)
				r.Put("/annotations/{qn}", s.upsertAnnotation)
				r.Delete("/annotations/{qn}", s.deleteAnnotation)
			})
		})
	})
	return r
}

// requireDB 中间件：无激活数据源时拒绝维度数据请求。
func (s *Server) requireDB(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.svc.ActiveSID() == "" {
			writeErr(w, 503, "no active source")
			return
		}
		next.ServeHTTP(w, r)
	})
}
