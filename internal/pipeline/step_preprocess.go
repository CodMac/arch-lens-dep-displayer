package pipeline

import (
	"context"
	"fmt"
	"os"

	"github.com/CodMac/arch-lens-dep-displayer/internal/db"
	"github.com/CodMac/arch-lens-dep-displayer/internal/db/repository"
	"github.com/CodMac/arch-lens-dep-displayer/internal/pipeline/preprocess"
)

// stepPreprocess 建维度/层级/预计算（结果预处理）。
func (p *Pipeline) stepPreprocess(ctx context.Context) (string, error) {
	c, err := db.Ensure(p.DBPath())
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close(c)

	dim, err := preprocess.Load(dimPath(p.language))
	if err != nil {
		return "", fmt.Errorf("load dimension: %w", err)
	}
	repo := repository.Of(c)
	levels := make([]repository.Level, 0, len(dim.Hierarchy))
	for _, lv := range dim.Hierarchy {
		levels = append(levels, repository.Level{Level: lv.Level, Role: lv.Role, Kinds: lv.Kinds})
	}
	dimID, err := repo.RegisterDimension(dim.Language, dim.QNSeparator,
		joinCSV(dim.MethodEdgeKinds()), levels, dim.RelationTypes, dim.StructuralRelationTypes)
	if err != nil {
		return "", fmt.Errorf("register dimension: %w", err)
	}
	dim.ID = dimID
	if err := preprocess.New(repo, dim).Build(); err != nil {
		return "", fmt.Errorf("build index: %w", err)
	}
	return fmt.Sprintf("维度=%s(id=%d) 层级/预计算完成", p.language, dimID), nil
}

// dimPath 返回维度配置路径（按语言选择，可被环境变量覆盖）。
func dimPath(language string) string {
	if v := os.Getenv("ARCH_LENS_DIM"); v != "" {
		return v
	}
	if language == "" {
		language = "java"
	}
	return "config/dimensions/" + language + ".yaml"
}
