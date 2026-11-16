// 维度配置模型（语言无关的纯模型，无任何 DB 依赖）：定义可下钻层级与 kind 白名单。
package preprocess

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// 层级角色（不随语言变）。
const (
	RoleNamespace = "namespace"
	RoleType      = "type"
	RoleMember    = "member"
)

// Level 定义维度的一个层级。
// Role = namespace | type | member；Kinds = 本层接受的 ElementKind 白名单。
type Level struct {
	Level int      `yaml:"level"`
	Role  string   `yaml:"role"`
	Kinds []string `yaml:"kinds"`
}

// Dimension 描述一种语言的可下钻层级视图。
type Dimension struct {
	Language                string   `yaml:"language"`
	QNSeparator             string   `yaml:"qn_separator"`
	Hierarchy               []Level  `yaml:"hierarchy"`
	RelationTypes           []string `yaml:"relation_types"`
	StructuralRelationTypes []string `yaml:"structural_relation_types"`
	// MethodKinds 方法级边两端 kind 白名单（可窄于 member 层，如 Java 不含 VARIABLE/ANONYMOUS_CLASS）。
	MethodKinds []string `yaml:"method_kinds"`
	ID          int      `yaml:"-"`
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
	if d.QNSeparator == "" {
		d.QNSeparator = "."
	}
	for _, l := range d.Hierarchy {
		switch l.Role {
		case RoleNamespace, RoleType, RoleMember:
		default:
			return nil, fmt.Errorf("dimension %s: level %d 的 role=%q 非法（应为 namespace|type|member）", path, l.Level, l.Role)
		}
		if len(l.Kinds) == 0 {
			return nil, fmt.Errorf("dimension %s: level %d 的 kinds 为空", path, l.Level)
		}
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

// LevelOfRole 返回指定角色对应的层级号，未定义返回 0。
func (d *Dimension) LevelOfRole(role string) int {
	for _, l := range d.Hierarchy {
		if l.Role == role {
			return l.Level
		}
	}
	return 0
}

// RoleOfLevel 返回指定层级的角色，未定义返回 ""。
func (d *Dimension) RoleOfLevel(level int) string {
	for _, l := range d.Hierarchy {
		if l.Level == level {
			return l.Role
		}
	}
	return ""
}

// KindsOfLevel 返回指定层级的 kind 白名单集合。
func (d *Dimension) KindsOfLevel(level int) map[string]bool {
	out := map[string]bool{}
	for _, l := range d.Hierarchy {
		if l.Level == level {
			for _, k := range l.Kinds {
				out[k] = true
			}
		}
	}
	return out
}

// KindsOfRole 返回指定角色的 kind 白名单集合。
func (d *Dimension) KindsOfRole(role string) map[string]bool {
	out := map[string]bool{}
	for _, l := range d.Hierarchy {
		if l.Role == role {
			for _, k := range l.Kinds {
				out[k] = true
			}
		}
	}
	return out
}

// MemberKinds 返回 member 层 kind 白名单。
func (d *Dimension) MemberKinds() []string {
	for _, l := range d.Hierarchy {
		if l.Role == RoleMember {
			return l.Kinds
		}
	}
	return nil
}

// MethodEdgeKinds 返回方法级边两端 kind 白名单（未配置时回退 member 层）。
func (d *Dimension) MethodEdgeKinds() []string {
	if len(d.MethodKinds) > 0 {
		return d.MethodKinds
	}
	return d.MemberKinds()
}

// StructuralTypes 返回结构关系类型（SQL IN 列表用）。
func (d *Dimension) StructuralTypes() []string {
	return d.StructuralRelationTypes
}

// RelationTypeList 返回依赖关系类型（SQL IN 列表用）。
func (d *Dimension) RelationTypeList() []string {
	return d.RelationTypes
}
