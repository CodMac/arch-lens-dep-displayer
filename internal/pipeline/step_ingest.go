package pipeline

import (
	"context"
	"fmt"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
	"github.com/CodMac/arch-lens-dep-displayer/internal/ingest"
)

// stepIngest 导入 JSONL → DuckDB。
func (p *Pipeline) stepIngest(ctx context.Context, pl *Pipeline) (string, error) {
	c, err := db.Open(p.DBPath())
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close(c)
	if err := db.Init(c); err != nil {
		return "", fmt.Errorf("init schema: %w", err)
	}
	ig := ingest.New(c, p.OutputDir())
	elCount, relCount, err := ig.Ingest()
	if err != nil {
		return "", fmt.Errorf("ingest: %w", err)
	}
	return fmt.Sprintf("元素=%d 关系=%d", elCount, relCount), nil
}
