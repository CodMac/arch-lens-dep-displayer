package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api/service"
	"github.com/go-chi/chi/v5"
)

// 响应与参数工具（不承载业务）。

func (s *Server) dimID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		return 0, false
	}
	return id, true
}

// ---- 数据源 - 薄壳 ----
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

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
