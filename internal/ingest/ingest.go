// Package ingest 提供原始数据导入器：把 dep-analyer 的 JSONL 输出导入 DuckDB 主表。
// 复用 arch-lens-dep-analyer 的 model 包，直接反序列化 CodeElement / DependencyRelation。
package ingest

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CodMac/arch-lens-dep-analyer/model"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
)

// Ingester 原始数据导入器：JSONL 产物 → elements/relations 主表。
// 生命周期短：每次分析按源执行一遍即弃，由 pipeline 使用。
type Ingester struct {
	db  *sql.DB
	dir string
}

// New 创建导入器。dir 为含 element.jsonl 与 relation_*.jsonl 的目录。
func New(c *sql.DB, dir string) *Ingester {
	return &Ingester{db: c, dir: dir}
}

// Ingest 执行完整导入：清空旧数据 → 扫描 JSONL → 批量写 DuckDB。
// 幂等：重复导入全量重建（对齐 middleware.md §8），避免元素/关系累积。
// 返回（元素数, 关系数）。
func (ig *Ingester) Ingest() (int, int, error) {
	if err := filepath.Walk(ig.dir, func(path string, info os.FileInfo, err error) error {
		return err // 透传错误
	}); err != nil {
		return 0, 0, fmt.Errorf("walk input dir: %w", err)
	}

	// 清空原始表（重建导入）。聚合/层级表由 index 阶段清空。
	if err := db.ClearRawTables(ig.db); err != nil {
		return 0, 0, err
	}

	elementPath := filepath.Join(ig.dir, "element.jsonl")
	elCount, err := ingestElements(ig.db, elementPath)
	if err != nil {
		return 0, 0, err
	}

	relCount := 0
	relPaths, err := relationFiles(ig.dir)
	if err != nil {
		return 0, 0, err
	}
	for _, p := range relPaths {
		n, err := ingestRelations(ig.db, p)
		if err != nil {
			return 0, 0, err
		}
		relCount += n
	}
	return elCount, relCount, nil
}

// relationFiles 列出输入目录下的 relation_*.jsonl 文件。
func relationFiles(dir string) ([]string, error) {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "relation_") && strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

// ingestElements 批量导入元素。
func ingestElements(c *sql.DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	tx, err := c.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO elements (qn, kind, name, path, is_source, is_sugar, is_external, start_line)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var count int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var el model.CodeElement
		if err := json.Unmarshal(line, &el); err != nil {
			return count, fmt.Errorf("element line %d: %w", count+1, err)
		}
		var sl int
		if el.Location != nil {
			sl = el.Location.StartLine
		}
		if _, err := stmt.Exec(el.QualifiedName, string(el.Kind), el.Name, el.Path, el.IsFormSource, el.IsFormSugar, el.IsFormExternal, sl); err != nil {
			return count, err
		}
		count++
	}
	if err := sc.Err(); err != nil {
		return count, err
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

// ingestRelations 批量导入单个关系文件。
func ingestRelations(c *sql.DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	tx, err := c.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO relations (source_qn, target_qn, type, start_line, raw_text)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var count int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r model.DependencyRelation
		if err := json.Unmarshal(line, &r); err != nil {
			return count, fmt.Errorf("relation line %d in %s: %w", count+1, filepath.Base(path), err)
		}
		var src, tgt, raw string
		var sl int
		if r.Source != nil {
			src = r.Source.QualifiedName
		}
		if r.Target != nil {
			tgt = r.Target.QualifiedName
		}
		if r.Location != nil {
			sl = r.Location.StartLine
		}
		if b, err := json.Marshal(r.Mores); err == nil && len(b) > 0 && string(b) != "null" {
			raw = string(b)
		}
		if _, err := stmt.Exec(src, tgt, string(r.Type), sl, raw); err != nil {
			return count, err
		}
		count++
	}
	if err := sc.Err(); err != nil {
		return count, err
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

var _ io.Reader // 保留 io 引用（bufio.Scanner 内部使用）
