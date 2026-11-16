# 语言特性文档

每种语言一份，定义该语言在**原始层 → 维度层 → 预计算层**的特化规则。通用规则见 [../development.md](../development.md)，表结构见 [../schema.md](../schema.md)。

| 语言 | 文档 | 状态 |
|---|---|---|
| Java | [java.md](java.md) | ✅ 已落地 |
| C++ | [cpp.md](cpp.md) | ✅ 已落地 |

## 新增一门语言

1. 新建 `config/dimensions/{lang}.yaml`，参考 [java.yaml](../../config/dimensions/java.yaml)：
   - `language` / `qn_separator`
   - `hierarchy`（`level` + `role` + `kinds`）
   - `relation_types`（依赖关系）
   - `structural_relation_types`（结构关系，不参与依赖边）
   - `method_kinds`（方法级边两端 kind 白名单）
2. 确认 analyzer 已按契约输出 `NamespaceQN` / `EnclosingQN` / `TypeQN`。
3. 新建 `docs/languages/{lang}.md`，按现有文档的 8 节结构写：
   维度配置、关系类型全集与端点 kind 矩阵、`type_edge` 口径、`call_site` / `method_edge`、`type_fan` / `violation_hit`、已知缺陷、环计算、验收基线。
4. 在本页表格登记。
