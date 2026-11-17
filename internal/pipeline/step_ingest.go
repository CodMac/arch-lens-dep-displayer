package pipeline

import (
	"context"
	"fmt"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline/ingest"
)

// stepIngest 导入 JSONL → DuckDB。
func (p *Pipeline) stepIngest(ctx context.Context) (string, error) {
	// 全量重建：删旧库文件 → 新库（含表约束），不做跨版本兼容。
	c, err := db.Recreate(p.DBPath())
	if err != nil {
		return "", fmt.Errorf("recreate db: %w", err)
	}
	defer db.Close(c)
	ig := ingest.New(repository.Of(c), p.OutputDir(), p.log)
	elCount, relCount, err := ig.Ingest()
	if err != nil {
		return "", fmt.Errorf("ingest: %w", err)
	}
	// 端点 JOIN 落空只计数告警，不改写（docs/development.md §2.3：预计算层不修补解析缺陷）。
	msg := fmt.Sprintf("元素=%d 关系=%d 悬空端点=%d", elCount, relCount, ig.Dangling)
	if ig.Malformed > 0 {
		msg += fmt.Sprintf(" 解析失败=%d", ig.Malformed)
	}
	if ig.Dangling > 0 || ig.Malformed > 0 {
		msg += "（详见 process.log）"
	}
	return msg, nil
}
