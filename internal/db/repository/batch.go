package repository

import (
	"fmt"
	"strings"
)

// batchSize 多行 VALUES INSERT 的单批行数（远快于逐行 execute）。
const batchSize = 200

// insertBatches 分批执行多行 VALUES INSERT，整体一个事务。
// 全仓唯一的批量写实现：ingest 与 preprocess 都走这里，避免各写一份。
func (r *Repository) insertBatches(prefix string, ncols int, rows [][]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	c := r.db()
	tx, err := c.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var sb strings.Builder
		sb.WriteString(prefix)
		sb.WriteString(" VALUES ")
		args := make([]interface{}, 0, len(chunk)*ncols)
		for i, row := range chunk {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(")
			for j := 0; j < ncols; j++ {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("?")
			}
			sb.WriteString(")")
			args = append(args, row...)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("batch insert: %w", err)
		}
	}
	return tx.Commit()
}
