package pipeline

import (
	"context"
	"fmt"
	"os"
)

// stepActivate 数据源就绪（由 registry 侧触发激活，此处仅标记）。
func (p *Pipeline) stepActivate(ctx context.Context, pl *Pipeline) (string, error) {
	if _, err := os.Stat(p.DBPath()); err != nil {
		return "", fmt.Errorf("lens.duckdb 不存在: %w", err)
	}
	return "数据源已就绪", nil
}
