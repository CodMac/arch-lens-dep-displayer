package pipeline

import (
	"context"
	"fmt"
	"os"
	"runtime"

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

// exportJSONL 将分析结果导出为 JSONL。
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
	return elCount, relCount, nil
}
