package repository

import "strings"

// sqlStringList 渲染受控字符串切片为 SQL 字面量列表（'A','B'）。
// 仅用于维度配置等受控值；用户输入必须走占位符。
func sqlStringList(vals []string) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, "'"+strings.ReplaceAll(v, "'", "''")+"'")
	}
	return strings.Join(parts, ",")
}

// arrayLiteral 渲染 DuckDB 数组字面量 ['A','B']。
func arrayLiteral(vals []string) string {
	return "[" + sqlStringList(vals) + "]"
}

// notInFilter 生成 "col NOT IN (...)"；空列表退化为恒真。
func notInFilter(col string, vals []string) string {
	if len(vals) == 0 {
		return "1=1"
	}
	return col + " NOT IN (" + sqlStringList(vals) + ")"
}

// inFilter 生成 "col IN (...)"；空列表退化为恒真。
func inFilter(col string, vals []string) string {
	if len(vals) == 0 {
		return "1=1"
	}
	return col + " IN (" + sqlStringList(vals) + ")"
}
