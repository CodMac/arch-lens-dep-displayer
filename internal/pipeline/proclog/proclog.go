// Package proclog 写数据源分析过程日志（sources/{sid}/process.log）。
//
// 用途：通过消费分析产物（analyze/export → ingest → preprocess）的过程日志，
// 发现 analyzer 产出的元素/关系是否有错误（悬空端点、归属错乱、游离节点、格式错误）。
// 约束：只读检查、不修补数据；写日志失败降级为 stderr，绝不拖垮分析。
package proclog

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxDetail 每个 stage 最多输出的明细条数（超出只写一行 truncated）。
const maxDetail = 200

// Logger 过程日志写入器。并发安全。
type Logger struct {
	mu       sync.Mutex
	f        *os.File
	path     string
	emitted  map[string]int // stage → 已写明细条数
	counts   map[string]int // stage → 明细总数（含被截断部分）
	warnOnce sync.Once
}

// Open 打开（截断重建）日志文件。path 为空或打开失败时返回一个只写 stderr 的降级 logger。
func Open(path string) *Logger {
	l := &Logger{path: path, emitted: map[string]int{}, counts: map[string]int{}}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		l.logStderr("open %s: %v", path, err)
		return l
	}
	l.f = f
	return l
}

// Path 返回日志文件路径。
func (l *Logger) Path() string { return l.path }

// Close 关闭文件（可重复调用）。
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Info 记录普通信息。
func (l *Logger) Info(stage, msg string, kv ...any) { l.write("INFO", stage, msg, kv...) }

// Warn 记录告警，并按 stage 限流（默认 200 条明细）。
func (l *Logger) Warn(stage, msg string, kv ...any) { l.writeLimited("WARN", stage, msg, kv...) }

// Error 记录错误，并按 stage 限流。
func (l *Logger) Error(stage, msg string, kv ...any) { l.writeLimited("ERROR", stage, msg, kv...) }

// Summary 记录汇总行（不受限流影响）。
func (l *Logger) Summary(stage string, kv ...any) {
	l.write("SUMMARY", stage, "", kv...)
}

// DetailCount 返回某 stage 已发现的明细总数（含被截断的）。
func (l *Logger) DetailCount(stage string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[stage]
}

// ---- 内部实现 ----

func (l *Logger) writeLimited(level, stage, msg string, kv ...any) {
	l.mu.Lock()
	l.counts[stage]++
	n := l.emitted[stage]
	if n >= maxDetail {
		l.mu.Unlock()
		return
	}
	l.emitted[stage] = n + 1
	l.mu.Unlock()
	l.write(level, stage, msg, kv...)
}

func (l *Logger) write(level, stage, msg string, kv ...any) {
	line := fmt.Sprintf("%s %-7s [%s]", time.Now().Format(time.RFC3339), level, stage)
	if msg != "" {
		line += " " + msg
	}
	if len(kv) > 0 {
		line += " " + formatKV(kv)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		l.logStderr("%s", line)
		return
	}
	if _, err := fmt.Fprintln(l.f, line); err != nil {
		l.logStderr("write %s: %v", l.path, err)
	}
}

// formatKV 把 key/value 对拼成 k=v（值含空白时加引号），按 key 排序保证稳定。
func formatKV(kv []any) string {
	type pair struct{ k, v string }
	pairs := make([]pair, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		k := fmt.Sprint(kv[i])
		v := fmt.Sprint(kv[i+1])
		if v == "" || strings.ContainsAny(v, " \t\"") {
			v = fmt.Sprintf("%q", v)
		}
		pairs = append(pairs, pair{k, v})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	return b.String()
}

func (l *Logger) logStderr(format string, args ...any) {
	l.warnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "proclog: "+format+"\n", args...)
	})
}
