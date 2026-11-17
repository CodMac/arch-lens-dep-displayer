// Package ingest 提供原始数据导入器：把 dep-analyer 的 JSONL 输出导入 DuckDB 主表。
// 复用 arch-lens-dep-analyer 的 model 包，直接反序列化 CodeElement / DependencyRelation。
// 本包只负责「JSONL → 行结构」的转换与文件遍历，所有 SQL 走 repository。
package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CodMac/arch-lens-dep-analyer/model"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline/proclog"
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
	log  *proclog.Logger
	// Dangling 端点 JOIN 落空计数（只告警，不改写；见 docs/development.md §2.3）。
	Dangling int
	// Malformed 解析失败被跳过的行数。
	Malformed int
}

// New 创建导入器。repo 绑定已建好表结构的连接，dir 为含 element.jsonl 与 relation_*.jsonl 的目录。
// log 可为 nil（不写过程日志）。
func New(repo *repository.Repository, dir string, log *proclog.Logger) *Ingester {
	if log == nil {
		log = proclog.Open("") // 降级：只写 stderr
	}
	return &Ingester{repo: repo, dir: dir, log: log}
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

	ig.reportAnomalies(elCount, relCount)
	return elCount, relCount, nil
}

// reportAnomalies 入库后做一致性检查并把明细/汇总写入过程日志（只读，不修补）。
func (ig *Ingester) reportAnomalies(elCount, relCount int) {
	log := ig.log

	// 1. 端点悬空（qn+kind 在 elements 中不存在）。
	total, list, err := ig.repo.DanglingRelations(200)
	if err != nil {
		log.Warn("ingest:dangling-endpoint", "", "err", err.Error())
	} else {
		ig.Dangling = total
		for _, d := range list {
			log.Warn("ingest:dangling-endpoint", "", "type", d.Type, "path", d.Path, "line", d.Line,
				"source_qn", d.SourceQN, "source_kind", d.SourceKind,
				"target_qn", d.TargetQN, "target_kind", d.TargetKind, "reason", d.Reason)
		}
		log.Summary("ingest:dangling-endpoint", "total", total, "emitted", len(list))
	}

	// 2. 端点 kind 不匹配（qn 存在但 kind 不同）。
	total, mism, err := ig.repo.KindMismatchRelations(200)
	if err != nil {
		log.Warn("ingest:kind-mismatch", "", "err", err.Error())
	} else {
		for _, m := range mism {
			log.Warn("ingest:kind-mismatch", "", "type", m.Type, "path", m.Path, "line", m.Line,
				"qn", m.QN, "expected_kind", m.Expected, "actual_kind", m.Actual, "end", m.End)
		}
		log.Summary("ingest:kind-mismatch", "total", total, "emitted", len(mism))
	}

	// 3. 关系类型分布（含结构关系）。
	if hist, err := ig.repo.RelationTypeHistogram(); err != nil {
		log.Warn("ingest:relation-types", "", "err", err.Error())
	} else {
		for _, kv := range sortedCounts(hist) {
			log.Info("ingest:relation-types", "", "type", kv.key, "count", kv.count)
		}
	}

	log.Summary("ingest", "elements", elCount, "relations", relCount,
		"malformed", ig.Malformed, "dangling", ig.Dangling)
}

// sortedCounts 把 map 转成按数量倒序、key 升序的切片。
func sortedCounts(m map[string]int) []struct {
	key   string
	count int
} {
	out := make([]struct {
		key   string
		count int
	}, 0, len(m))
	for k, v := range m {
		out = append(out, struct {
			key   string
			count int
		}{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].key < out[j].key
	})
	return out
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
			// 记录并跳过（不中断整库导入），让问题暴露在日志里。
			ig.Malformed++
			ig.log.Error("ingest:malformed-json", "", "file", filepath.Base(path), "line", count+1, "err", err.Error())
			count++
			return nil
		}
		row := toElementRow(el)
		if el.QualifiedName == "" {
			ig.log.Warn("ingest:empty-element-qn", "", "file", filepath.Base(path), "line", count+1, "kind", string(el.Kind))
		}
		key := el.QualifiedName + "\x00" + string(el.Kind)
		if i, ok := seen[key]; ok {
			ig.log.Warn("ingest:duplicate-element", "", "qn", el.QualifiedName, "kind", string(el.Kind),
				"file", filepath.Base(path), "line", count+1, "action", "overwrite")
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
	base := filepath.Base(path)
	err = scanJSONL(f, func(line []byte) error {
		var rel model.DependencyRelation
		if err := json.Unmarshal(line, &rel); err != nil {
			ig.Malformed++
			ig.log.Error("ingest:malformed-json", "", "file", base, "line", count+1, "err", err.Error())
			count++
			return nil
		}
		row := toRelationRow(rel)
		// 空字段告警（不阻断导入）。
		var missing []string
		if row.SourceQN == "" {
			missing = append(missing, "source_qn")
		}
		if row.TargetQN == "" {
			missing = append(missing, "target_qn")
		}
		if row.Type == "" {
			missing = append(missing, "type")
		}
		if len(missing) > 0 {
			ig.log.Warn("ingest:empty-field", "", "file", base, "line", count+1,
				"missing", strings.Join(missing, ","), "type", row.Type,
				"source_qn", row.SourceQN, "target_qn", row.TargetQN)
		}
		rows = append(rows, row)
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
