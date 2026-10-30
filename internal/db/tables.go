package db

import (
	"database/sql"
	"fmt"
)

// ---- 原始表操作（主表驱动） ----

// ClearRawTables 清空原始表 elements/relations（重建导入，防累积）。
func ClearRawTables(c *sql.DB) error {
	for _, q := range []string{`DELETE FROM relations`, `DELETE FROM elements`} {
		if _, err := c.Exec(q); err != nil {
			return fmt.Errorf("clear raw tables: %w", err)
		}
	}
	return nil
}

// InsertElement 写入单个元素行。
func InsertElement(c *sql.DB, qn, kind, name, path string, isSource, isSugar, isExternal bool, startLine int) error {
	_, err := c.Exec(`INSERT INTO elements (qn, kind, name, path, is_source, is_sugar, is_external, start_line) VALUES (?,?,?,?,?,?,?,?)`,
		qn, kind, name, path, isSource, isSugar, isExternal, startLine)
	return err
}

// InsertRelation 写入单个关系行。
func InsertRelation(c *sql.DB, sourceQN, targetQN, relType string, startLine int, rawText string) error {
	_, err := c.Exec(`INSERT INTO relations (source_qn, target_qn, type, start_line, raw_text) VALUES (?,?,?,?,?)`,
		sourceQN, targetQN, relType, startLine, rawText)
	return err
}

// ---- 维度表操作 ----

// DimensionID 返回指定语言的维度 id（不存在返回 0）。
func DimensionID(c *sql.DB, language string) int {
	var id int
	_ = c.QueryRow(`SELECT id FROM dimensions WHERE language = ?`, language).Scan(&id)
	return id
}

// NextDimensionID 返回下一个可用的维度 id（MAX+1，DuckDB PRIMARY KEY 不自增）。
func NextDimensionID(c *sql.DB) int {
	var maxID int
	_ = c.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM dimensions`).Scan(&maxID)
	return maxID + 1
}

// InsertDimension 写入维度行。
func InsertDimension(c *sql.DB, id int, language, name string) error {
	_, err := c.Exec(`INSERT INTO dimensions (id, language, name) VALUES (?, ?, ?)`, id, language, name)
	return err
}

// ClearDimensionLevels 清空某维度的层级定义。
func ClearDimensionLevels(c *sql.DB, dimID int) error {
	_, err := c.Exec(`DELETE FROM dimension_levels WHERE dimension_id = ?`, dimID)
	return err
}

// InsertDimensionLevel 写入维度层级定义。
func InsertDimensionLevel(c *sql.DB, dimID, level int, kind, parentOf string) error {
	_, err := c.Exec(`INSERT INTO dimension_levels (dimension_id, level, kind, parent_of) VALUES (?, ?, ?, ?)`,
		dimID, level, kind, parentOf)
	return err
}

// ---- 标注表操作 ----

// UpsertAnnotation 收藏/备注 upsert（部分更新：starred/note 传 nil 表示不修改）。
func UpsertAnnotation(c *sql.DB, dimID int, qn string, starred *bool, note *string) error {
	_, err := c.Exec(`INSERT INTO annotations (dimension_id, qn, starred, note) VALUES (?,?,?,?)
		ON CONFLICT (dimension_id, qn) DO UPDATE SET
			starred = COALESCE(?, starred),
			note = COALESCE(?, note),
			updated_at = now()`,
		dimID, qn, starredOrFalse(starred), noteOrEmpty(note), starred, note)
	return err
}

// DeleteAnnotation 删除标注行（取消收藏）。
func DeleteAnnotation(c *sql.DB, dimID int, qn string) error {
	_, err := c.Exec(`DELETE FROM annotations WHERE dimension_id = ? AND qn = ?`, dimID, qn)
	return err
}

// starredOrFalse 展开 *bool：nil → false。
func starredOrFalse(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// noteOrEmpty 展开 *string：nil → ""。
func noteOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
