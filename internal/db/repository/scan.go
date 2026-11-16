package repository

import (
	"strings"
)

func lastSeg(qn string) string {
	if i := strings.LastIndexAny(qn, ".:"); i >= 0 {
		return qn[i+1:]
	}
	return qn
}

func intersects(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if set[v] {
			return true
		}
	}
	return false
}

// SplitTypes 解析逗号分隔的 kind 白名单（dimension_levels.kinds）。
func SplitTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SplitArrayString 解析 DuckDB 数组列的字符串表示（"[CALL, USE]" → ["CALL","USE"]）。
func SplitArrayString(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
