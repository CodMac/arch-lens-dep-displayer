// Package pipeline 抽象「源码 → 可浏览数据源」的完整处理链（面向对象步骤化）。
// 每个步骤是独立的 Step 对象，串行执行并记录耗时/状态；状态持久化到 meta.json，
// 支持前端轮询进度与断点续跑（Resume）。
//
// 步骤顺序（约定）：scan → analyze → export → ingest → preprocess → activate。
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StepID 步骤标识（约定顺序）。
type StepID string

const (
	StepScan       StepID = "scan"       // 扫描源码文件
	StepAnalyze    StepID = "analyze"    // 依赖分析（内嵌 analyzer）
	StepExport     StepID = "export"     // 导出 JSONL
	StepIngest     StepID = "ingest"     // JSONL → DuckDB
	StepPreprocess StepID = "preprocess" // 维度/层级/预计算
	StepActivate   StepID = "activate"   // 数据源激活（就绪）
)

// AllSteps 约定好的完整步骤序列。
var AllSteps = []StepID{StepScan, StepAnalyze, StepExport, StepIngest, StepPreprocess, StepActivate}

// StepLabels 步骤中文名（前端展示用）。
var StepLabels = map[StepID]string{
	StepScan:       "扫描源码",
	StepAnalyze:    "依赖分析",
	StepExport:     "导出 JSONL",
	StepIngest:     "导入 DuckDB",
	StepPreprocess: "结果预处理",
	StepActivate:   "数据源就绪",
}

// StepStatus 步骤状态。
type StepStatus string

const (
	StatusPending StepStatus = "pending"
	StatusRunning StepStatus = "running"
	StatusDone    StepStatus = "done"
	StatusFailed  StepStatus = "failed"
	StatusSkipped StepStatus = "skipped"
)

// StepResult 单个步骤的耗时/状态快照（供前端 status 轮询）。
type StepResult struct {
	ID       StepID     `json:"id"`
	Label    string     `json:"label"`
	Status   StepStatus `json:"status"`
	Duration float64    `json:"duration"` // 秒
	Message  string     `json:"message,omitempty"`
}

// stepFunc 步骤执行体：返回展示用 message。
type stepFunc func(ctx context.Context, p *Pipeline) (string, error)

// Step 一个可执行步骤。
type Step struct {
	ID  StepID
	Run stepFunc
}

// Pipeline 编排器：持有一组 Step，串行执行，记录每步耗时/状态并持久化到 meta.json。
// 生命周期短：单源一次分析链，跑完即释放（由 registry 按源 new 一个）。
type Pipeline struct {
	sourcePath string
	language   string
	workspace  string // sources/{sid}
	steps      []*Step
	results    []StepResult
	mu         sync.Mutex
}

// MetaFile 是 pipeline 持久化到 meta.json 的载体。
type MetaFile struct {
	SourcePath string       `json:"path"`
	Language   string       `json:"language"`
	Status     string       `json:"status"`
	Steps      []StepResult `json:"steps"`
	CreatedAt  string       `json:"created_at"`
}

// New 按约定顺序构造 pipeline（6 个 Step）。
// wsDir 为该数据源的工作目录（sources/{sid}），内含 output/ 与 lens.duckdb。
func New(sourcePath, language, wsDir string) *Pipeline {
	p := &Pipeline{
		sourcePath: sourcePath,
		language:   language,
		workspace:  wsDir,
		results:    make([]StepResult, 0, len(AllSteps)),
	}
	for _, id := range AllSteps {
		p.results = append(p.results, StepResult{ID: id, Label: StepLabels[id], Status: StatusPending})
	}
	p.steps = []*Step{
		{ID: StepScan, Run: p.stepScan},
		{ID: StepAnalyze, Run: p.stepAnalyze},
		{ID: StepExport, Run: p.stepExport},
		{ID: StepIngest, Run: p.stepIngest},
		{ID: StepPreprocess, Run: p.stepPreprocess},
		{ID: StepActivate, Run: p.stepActivate},
	}
	return p
}

// SourcePath 返回被分析源码路径。
func (p *Pipeline) SourcePath() string { return p.sourcePath }

// Language 返回分析语言。
func (p *Pipeline) Language() string { return p.language }

// Workspace 返回该数据源工作目录。
func (p *Pipeline) Workspace() string { return p.workspace }

// Results 返回步骤快照副本（读安全）。
func (p *Pipeline) Results() []StepResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]StepResult, len(p.results))
	copy(out, p.results)
	return out
}

// Status 返回整体状态：全部 done=ready；任一 failed=failed；否则 analyzing。
func (p *Pipeline) Status() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.statusLocked()
}

func (p *Pipeline) statusLocked() string {
	done := 0
	for _, r := range p.results {
		if r.Status == StatusFailed {
			return "failed"
		}
		if r.Status == StatusDone {
			done++
		}
	}
	if done == len(p.results) {
		return "ready"
	}
	return "analyzing"
}

// DBPath 返回该数据源 DuckDB 文件路径。
func (p *Pipeline) DBPath() string {
	return filepath.Join(p.workspace, "lens.duckdb")
}

// OutputDir 返回 JSONL 输出目录。
func (p *Pipeline) OutputDir() string {
	return filepath.Join(p.workspace, "output")
}

// Run 串行执行全部步骤。可被 Resume 复用（跳过已 done 的步骤）。
func (p *Pipeline) Run(ctx context.Context) error {
	for _, st := range p.steps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := p.runStep(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// Resume 从 meta.json 恢复已完成的步骤，只跑未完成的（断点续跑）。
func (p *Pipeline) Resume(ctx context.Context) error {
	if mf, err := p.loadMeta(); err == nil {
		p.mu.Lock()
		for i := range p.results {
			if i < len(mf.Steps) && mf.Steps[i].ID == p.results[i].ID {
				p.results[i] = mf.Steps[i]
			}
		}
		p.mu.Unlock()
	}
	return p.Run(ctx)
}

// runStep 执行单个步骤：置 running → 执行 → 置 done/failed，写 meta.json。
func (p *Pipeline) runStep(ctx context.Context, st *Step) error {
	p.mu.Lock()
	if p.resultByID(st.ID).Status == StatusDone {
		p.mu.Unlock()
		return nil // 已 done（Resume 场景）跳过
	}
	p.setStatus(st.ID, StatusRunning, "")
	p.persistLocked()
	p.mu.Unlock()

	start := time.Now()
	msg, err := st.Run(ctx, p)
	dur := time.Since(start).Seconds()

	p.mu.Lock()
	if err != nil {
		p.setStatus(st.ID, StatusFailed, err.Error())
		p.persistLocked()
		p.mu.Unlock()
		return fmt.Errorf("step %s: %w", st.ID, err)
	}
	p.setStatus(st.ID, StatusDone, msg)
	p.setDuration(st.ID, dur)
	p.persistLocked()
	p.mu.Unlock()
	return nil
}

// setStatus 设置某步骤状态并更新耗时（须持锁调用）。
func (p *Pipeline) setStatus(id StepID, st StepStatus, msg string) {
	for i := range p.results {
		if p.results[i].ID == id {
			p.results[i].Status = st
			p.results[i].Message = msg
		}
	}
}

// setDuration 记录步骤耗时（须持锁调用）。
func (p *Pipeline) setDuration(id StepID, dur float64) {
	for i := range p.results {
		if p.results[i].ID == id {
			p.results[i].Duration = dur
		}
	}
}

func (p *Pipeline) resultByID(id StepID) *StepResult {
	for i := range p.results {
		if p.results[i].ID == id {
			return &p.results[i]
		}
	}
	return &p.results[0]
}

// ---- meta.json 持久化 ----

func (p *Pipeline) metaPath() string {
	return filepath.Join(p.workspace, "meta.json")
}

// persistLocked 将当前状态写回 meta.json（须持锁调用）。
func (p *Pipeline) persistLocked() {
	if err := os.MkdirAll(p.workspace, 0o755); err != nil {
		return
	}
	mf := MetaFile{
		SourcePath: p.sourcePath,
		Language:   p.language,
		Status:     p.statusLocked(),
		Steps:      p.results,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p.metaPath(), b, 0o644)
}

// loadMeta 读取 meta.json（用于 Resume）。
func (p *Pipeline) loadMeta() (*MetaFile, error) {
	b, err := os.ReadFile(p.metaPath())
	if err != nil {
		return nil, err
	}
	var mf MetaFile
	if err := json.Unmarshal(b, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}
