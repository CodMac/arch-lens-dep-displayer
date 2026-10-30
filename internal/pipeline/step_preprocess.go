package pipeline

import (
	"context"
	"fmt"
	"os"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
	"github.com/CodMac/arch-lens-dep-displayer/internal/dimension"
	"github.com/CodMac/arch-lens-dep-displayer/internal/index"
)

// stepPreprocess 建维度/层级/预计算（结果预处理）。
func (p *Pipeline) stepPreprocess(ctx context.Context, pl *Pipeline) (string, error) {
	c, err := db.Open(p.DBPath())
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close(c)
	if err := db.Init(c); err != nil {
		return "", fmt.Errorf("init schema: %w", err)
	}

	dim, err := dimension.Load(dimPath())
	if err != nil {
		return "", fmt.Errorf("load dimension: %w", err)
	}
	dimID, err := registerDimension(c, dim, p.language)
	if err != nil {
		return "", fmt.Errorf("register dimension: %w", err)
	}
	dim.ID = dimID
	ix := index.New(c, dim)
	if err := ix.Build(); err != nil {
		return "", fmt.Errorf("build index: %w", err)
	}
	return fmt.Sprintf("维度=%s(id=%d) 层级/预计算完成", p.language, dimID), nil
}

// dimPath 返回维度配置路径（相对项目根，可被环境变量覆盖）。
func dimPath() string {
	if v := os.Getenv("ARCH_LENS_DIM"); v != "" {
		return v
	}
	return "config/dimensions/java.yaml"
}
