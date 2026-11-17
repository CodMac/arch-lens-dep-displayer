package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/CodMac/arch-lens-dep-analyer/core"
	"github.com/CodMac/arch-lens-dep-analyer/engine"
	"github.com/CodMac/arch-lens-dep-analyer/model"
	"github.com/CodMac/arch-lens-dep-analyer/output"
	_ "github.com/CodMac/arch-lens-dep-analyer/x/java" // 注册 Java 语言实现
)

// stepAnalyze 依赖分析 + 导出 JSONL（单步完成，避免重复遍历/重复分析）。
func (p *Pipeline) stepAnalyze(ctx context.Context) (string, error) {
	ec, err := p.analyzeOnce()
	if err != nil {
		return "", err
	}
	elCount, relCount, err := p.exportJSONL(ec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("元素=%d 关系=%d", elCount, relCount), nil
}

// analyzeOnce 运行内嵌 analyzer 引擎并缓存 EngineContext（同一次运行内只分析一遍）。
func (p *Pipeline) analyzeOnce() (*engine.EngineContext, error) {
	if p.ec != nil {
		return p.ec, nil
	}
	impl, err := core.Get(core.Language(p.language))
	if err != nil {
		return nil, fmt.Errorf("获取语言实现失败: %w", err)
	}
	files, err := p.sourceFiles()
	if err != nil {
		return nil, err
	}
	ec, err := engine.RunDefault(engine.Config{
		Language:    core.Language(p.language),
		RootPath:    p.sourcePath,
		FilePaths:   files,
		Concurrency: runtime.NumCPU(),
		FilterLevel: core.FilterLevel(1), // Balanced
	}, func() (core.Parser, error) {
		return impl.Parser.NewParser(), nil
	})
	if err != nil {
		return nil, fmt.Errorf("分析执行失败: %w", err)
	}
	p.ec = ec
	return ec, nil
}

// exportJSONL 将分析结果导出为 JSONL，并把产物统计写入过程日志。
func (p *Pipeline) exportJSONL(ec *engine.EngineContext) (int, int, error) {
	outDir := p.OutputDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, 0, err
	}
	defs := ec.GC.Definitions()
	elements := make([]*model.CodeElement, 0, len(defs))
	for _, def := range defs {
		elements = append(elements, def.Element)
	}
	elCount, relCount, err := output.NewExporter(outDir).ExportJsonL(elements, ec.Relations)
	if err != nil {
		return 0, 0, fmt.Errorf("导出失败: %w", err)
	}
	p.reportArtifacts(elements, ec.Relations)
	return elCount, relCount, nil
}

// reportArtifacts 记录分析产物的规模、分布与可疑项（只读检查，不修补）。
func (p *Pipeline) reportArtifacts(elements []*model.CodeElement, relations []*model.DependencyRelation) {
	log := p.log

	// 1. 规模与字段覆盖率。
	var emptyQN, emptyNS, emptyEnclosing, emptyType, sugar, implDetail int
	kindCount := map[string]int{}
	for _, el := range elements {
		if el == nil {
			continue
		}
		kindCount[string(el.Kind)]++
		if el.QualifiedName == "" {
			emptyQN++
		}
		if el.NamespaceQN == "" {
			emptyNS++
		}
		if el.EnclosingQN == "" {
			emptyEnclosing++
		}
		if el.TypeQN == "" {
			emptyType++
		}
		if el.IsFormSugar {
			sugar++
		}
		if implDetailKind(el.Kind) {
			implDetail++
		}
	}
	log.Summary("analyze", "elements", len(elements), "relations", len(relations),
		"empty_qn", emptyQN, "empty_namespace", emptyNS,
		"empty_enclosing", emptyEnclosing, "empty_type", emptyType,
		"sugar", sugar, "impl_detail", implDetail)

	// 2. 元素 kind 分布（按数量倒序）。
	for _, kv := range sortedCounts(kindCount) {
		log.Info("analyze:kinds", "", "kind", kv.key, "count", kv.count)
	}

	// 3. 关系类型直方图 + 空端点检查。
	relTypes := map[string]int{}
	var nilEndpoint, emptyEndpoint int
	for _, rel := range relations {
		if rel == nil {
			continue
		}
		relTypes[string(rel.Type)]++
		if rel.Source == nil || rel.Target == nil {
			nilEndpoint++
			log.Warn("analyze:null-endpoint", "", "type", string(rel.Type),
				"source_nil", rel.Source == nil, "target_nil", rel.Target == nil)
			continue
		}
		if rel.Source.QualifiedName == "" || rel.Target.QualifiedName == "" {
			emptyEndpoint++
			log.Warn("analyze:empty-endpoint", "", "type", string(rel.Type),
				"source_qn", rel.Source.QualifiedName, "target_qn", rel.Target.QualifiedName)
		}
	}
	for _, kv := range sortedCounts(relTypes) {
		log.Info("analyze:relations", "", "type", kv.key, "count", kv.count)
	}
	log.Summary("analyze:relations", "types", len(relTypes),
		"null_endpoint", nilEndpoint, "empty_endpoint", emptyEndpoint)

	// 4. 未知元素 kind（不在维度配置白名单内，且不是 FILE/PACKAGE）。
	if allowed, ok := p.dimensionKinds(); ok {
		unknown := map[string]int{}
		for _, el := range elements {
			if el == nil || el.Kind == "FILE" || el.Kind == "PACKAGE" {
				continue
			}
			if !allowed[string(el.Kind)] {
				unknown[string(el.Kind)]++
			}
		}
		for _, kv := range sortedCounts(unknown) {
			log.Warn("analyze:unknown-kind", "", "kind", kv.key, "count", kv.count)
		}
		if len(unknown) > 0 {
			log.Summary("analyze:unknown-kind", "kinds", len(unknown))
		}
	}

	// 5. 导出产物清单（行数 + 字节数）。
	entries, err := os.ReadDir(p.OutputDir())
	if err != nil {
		log.Warn("export:list", "", "err", err.Error())
		return
	}
	var totalBytes int64
	files := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files++
		totalBytes += info.Size()
		lines := countLines(filepath.Join(p.OutputDir(), e.Name()))
		log.Info("export:file", "", "file", e.Name(), "lines", lines, "bytes", info.Size())
	}
	log.Summary("export", "files", files, "bytes", totalBytes)
}

// dimensionKinds 读取维度配置的全部 kind 白名单（用于未知 kind 告警）。
func (p *Pipeline) dimensionKinds() (map[string]bool, bool) {
	dim, err := loadDimensionConfig(dimPath(p.language))
	if err != nil {
		p.log.Warn("analyze:dimension", "", "err", err.Error())
		return nil, false
	}
	allowed := map[string]bool{}
	for _, lv := range dim.Hierarchy {
		for _, k := range lv.Kinds {
			allowed[k] = true
		}
	}
	return allowed, true
}

// implDetailKind 源码语法结构（与 is_sugar 正交）。
func implDetailKind(kind model.ElementKind) bool {
	switch kind {
	case model.Lambda, model.AnonymousClass, model.MethodRef, model.ScopeBlock:
		return true
	}
	return false
}

type keyCount struct {
	key   string
	count int
}

// sortedCounts 把 map 转成按数量倒序、key 升序的切片。
func sortedCounts(m map[string]int) []keyCount {
	out := make([]keyCount, 0, len(m))
	for k, v := range m {
		out = append(out, keyCount{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].key < out[j].key
	})
	return out
}

// countLines 统计文件行数。
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	lines := 0
	for {
		k, err := f.Read(buf)
		for i := 0; i < k; i++ {
			if buf[i] == '\n' {
				lines++
			}
		}
		if err != nil {
			return lines
		}
	}
}
