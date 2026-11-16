package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// stepScan 扫描源码目录下的源文件。
func (p *Pipeline) stepScan(ctx context.Context) (string, error) {
	files, err := p.sourceFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("路径 %s 下无匹配 .%s 文件", p.sourcePath, p.language)
	}
	return fmt.Sprintf("找到 %d 个源文件", len(files)), nil
}

// sourceFiles 返回源文件列表（首次扫描后缓存，供 analyze 复用）。
func (p *Pipeline) sourceFiles() ([]string, error) {
	if p.files != nil {
		return p.files, nil
	}
	files, err := scanFiles(p.sourcePath, p.language)
	if err != nil {
		return nil, err
	}
	p.files = files
	return files, nil
}

// scanFiles 按正则过滤扫描目录下的源文件（与 analyzer CLI 一致）。
func scanFiles(rootPath, lang string) ([]string, error) {
	re, err := regexp.Compile(fmt.Sprintf(`.*\.%s$`, lang))
	if err != nil {
		return nil, err
	}
	absRoot, _ := filepath.Abs(rootPath)
	var files []string
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && re.MatchString(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
