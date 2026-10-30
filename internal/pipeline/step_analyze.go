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

// analyzeOnce 运行内嵌 analyzer 引擎并返回 EngineContext。
func (p *Pipeline) analyzeOnce() (*engine.EngineContext, error) {
	impl, err := core.Get(core.Language(p.language))
	if err != nil {
		return nil, fmt.Errorf("获取语言实现失败: %w", err)
	}
	files, err := scanFiles(p.sourcePath, p.language)
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
	return ec, nil
}

// stepAnalyze 运行内嵌 analyzer 引擎。
func (p *Pipeline) stepAnalyze(ctx context.Context, pl *Pipeline) (string, error) {
	ec, err := p.analyzeOnce()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("元素=%d 关系=%d", len(ec.GC.Definitions()), len(ec.Relations)), nil
}

// stepExport 将分析结果导出为 JSONL。
func (p *Pipeline) stepExport(ctx context.Context, pl *Pipeline) (string, error) {
	ec, err := p.analyzeOnce()
	if err != nil {
		return "", err
	}
	outDir := p.OutputDir()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	exporter := output.NewExporter(outDir)
	defs := ec.GC.Definitions()
	elements := make([]*model.CodeElement, 0, len(defs))
	for _, def := range defs {
		elements = append(elements, def.Element)
	}
	elCount, relCount, err := exporter.ExportJsonL(elements, ec.Relations)
	if err != nil {
		return "", fmt.Errorf("导出失败: %w", err)
	}
	return fmt.Sprintf("元素=%d 关系=%d", elCount, relCount), nil
}
