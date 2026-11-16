// Package ingest 提供原始数据导入器：把 dep-analyer 的 JSONL 输出导入 DuckDB 主表。
// 复用 arch-lens-dep-analyer 的 model 包，直接反序列化 CodeElement / DependencyRelation。
// 本包只负责「JSONL → 行结构」的转换与文件遍历，所有 SQL 走 repository。
package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CodMac/arch-lens-dep-analyer/model"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
)

// implDetailKinds 源码语法结构（L2「隐藏实现细节」开关用），与 is_sugar 正交（决策 4）。
var implDetailKinds = map[model.ElementKind]bool{
	model.Lambda:         true,
	model.AnonymousClass: true,
	model.MethodRef:      true,
	model.ScopeBlock:     true,
}

// Ingester 原始数据导入器：JSONL 产物 → elements/relations 主表。
// 生命周期短：每次分析按源执行一遍即弃，由 pipeline 使用。
type Ingester struct {
	repo *repository.Repository
	dir  string
	// Dangling 端点 JOIN 落空计数（只告警，不改写；见 §11 决策 22）。
	Dangling int
}

// New 创建导入器。repo 绑定已建好表结构的连接，dir 为含 element.jsonl 与 relation_*.jsonl 的目录。
func New(repo *repository.Repository, dir string) *Ingester {
	return &Ingester{repo: repo, dir: dir}
}

// Ingest 执行完整导入：扫描 JSONL → 批量写 DuckDB。
// 幂等：调用方已用 db.Recreate 建空库（整库重建），无需清表。返回（元素数, 关系数）。
func (ig *Ingester) Ingest() (int, int, error) {
	elCount, err := ig.ingestElements(filepath.Join(ig.dir, "element.jsonl"))
	if err != nil {
		return 0, 0, err
	}

	relPaths, err := relationFiles(ig.dir)
	if err != nil {
		return 0, 0, err
	}
	relCount := 0
	for _, p := range relPaths {
		n, err := ig.ingestRelations(p)
		if err != nil {
			return 0, 0, err
		}
		relCount += n
	}

	n, err := ig.repo.CountDanglingRelations()
	if err != nil {
		return 0, 0, err
	}
	ig.Dangling = n
	return elCount, relCount, nil
}

// ---- 对象方法：单文件导入 ----

// ingestElements 批量导入元素。
// 主键 (qn, kind)：先在 Go 侧去重（同键后出现者覆盖先出现者，即定义覆盖声明）。
func (ig *Ingester) ingestElements(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	seen := map[string]int{}
	var rows []repository.ElementRow
	var count int
	err = scanJSONL(f, func(line []byte) error {
		var el model.CodeElement
		if err := json.Unmarshal(line, &el); err != nil {
			return fmt.Errorf("element line %d: %w", count+1, err)
		}
		row := toElementRow(el)
		key := el.QualifiedName + "\x00" + string(el.Kind)
		if i, ok := seen[key]; ok {
			rows[i] = row
		} else {
			seen[key] = len(rows)
			rows = append(rows, row)
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	return count, ig.repo.InsertElements(rows)
}

// ingestRelations 批量导入单个关系文件。
func (ig *Ingester) ingestRelations(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var rows []repository.RelationRow
	var count int
	err = scanJSONL(f, func(line []byte) error {
		var rel model.DependencyRelation
		if err := json.Unmarshal(line, &rel); err != nil {
			return fmt.Errorf("relation line %d in %s: %w", count+1, filepath.Base(path), err)
		}
		rows = append(rows, toRelationRow(rel))
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	return count, ig.repo.InsertRelations(rows)
}

// ---- 辅助函数：文件遍历与模型转换 ----

// scanJSONL 逐行扫描 JSONL（跳过空行），每行交给 fn。
// 统一处理缓冲区大小与空行过滤，避免每个导入函数各写一遍。
func scanJSONL(f *os.File, fn func(line []byte) error) error {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return sc.Err()
}

// relationFiles 列出输入目录下的 relation_*.jsonl 文件。
func relationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "relation_") && strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

// toElementRow analyzer 模型 → 元素表写入行。
func toElementRow(el model.CodeElement) repository.ElementRow {
	row := repository.ElementRow{
		QN: el.QualifiedName, Kind: string(el.Kind), Name: el.Name, Path: el.Path,
		Signature: el.Signature, Doc: el.Doc, Comment: el.Comment,
		NamespaceQN: el.NamespaceQN, EnclosingQN: el.EnclosingQN, TypeQN: el.TypeQN,
		IsSource: el.IsFormSource, IsSugar: el.IsFormSugar,
		IsImplDetail: implDetailKinds[el.Kind],
		Extra:        nullJSON(marshalJSON(el.Extra)),
	}
	if el.Location != nil {
		row.StartLine = el.Location.StartLine
		row.EndLine = el.Location.EndLine
	}
	return row
}

// toRelationRow analyzer 模型 → 关系表写入行。
func toRelationRow(rel model.DependencyRelation) repository.RelationRow {
	row := repository.RelationRow{Type: string(rel.Type), RawText: nullJSON(marshalJSON(rel.Mores))}
	if rel.Source != nil {
		row.SourceQN = rel.Source.QualifiedName
		row.SourceKind = string(rel.Source.Kind)
	}
	if rel.Target != nil {
		row.TargetQN = rel.Target.QualifiedName
		row.TargetKind = string(rel.Target.Kind)
	}
	if rel.Location != nil {
		row.StartLine = rel.Location.StartLine
		row.EndLine = rel.Location.EndLine
		row.Path = rel.Location.FilePath
	}
	return row
}

// marshalJSON 序列化可选字段；nil 或 "null" 返回空串。
func marshalJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}

// nullJSON 空字符串转 NULL（DuckDB JSON 列不接受空串）。
func nullJSON(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
