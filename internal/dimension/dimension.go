// Package dimension 定义可下钻层级维度配置（语言无关的纯模型，无任何 DB 依赖）。
package dimension

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Level 定义维度的一个层级。
type Level struct {
	Level    int      `yaml:"level"`
	Kind     string   `yaml:"kind"`
	ParentOf []string `yaml:"parent_of"`
}

// Dimension 描述一种语言的可下钻层级视图。
type Dimension struct {
	Language      string   `yaml:"language"`
	Hierarchy     []Level  `yaml:"hierarchy"`
	RelationTypes []string `yaml:"relation_types"`
	ID            int      `yaml:"-"`
}

// Load 从 YAML 解析维度配置。
func Load(path string) (*Dimension, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dimension %s: %w", path, err)
	}
	var d Dimension
	if err := yaml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse dimension %s: %w", path, err)
	}
	if d.Language == "" || len(d.Hierarchy) == 0 {
		return nil, fmt.Errorf("dimension %s: language/hierarchy 缺失", path)
	}
	return &d, nil
}

// MaxLevel 返回最深层级。
func (d *Dimension) MaxLevel() int {
	mx := 0
	for _, l := range d.Hierarchy {
		if l.Level > mx {
			mx = l.Level
		}
	}
	return mx
}

// LevelKind 返回指定层级的 kind，未定义返回 ""。
func (d *Dimension) LevelKind(level int) string {
	for _, l := range d.Hierarchy {
		if l.Level == level {
			return l.Kind
		}
	}
	return ""
}

// ParentLevel 返回 level 的父层级（level-1，若存在）。
func (d *Dimension) ParentLevel(level int) int {
	if level <= 1 {
		return 0
	}
	return level - 1
}

// HasRelationType 判断关系类型是否参与分析。
func (d *Dimension) HasRelationType(t string) bool {
	for _, rt := range d.RelationTypes {
		if rt == t {
			return true
		}
	}
	return false
}
