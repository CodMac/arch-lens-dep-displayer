# C++ 语言口径

> 本文定义 C++ 在**原始层 → 维度层 → 预计算层**的语言特化规则。通用规则见 [development.md](../development.md)，表结构见 [schema.md](../schema.md)。
> 数据基线：analyzer `00df5f3`，`x/cpp/testdata/collector/cpp`（27 文件 / 739 元素 / 1,145 关系）与 `decl_def` 样本（2026-11-13）。

---

## 1. 维度配置（`dimensions` / `dimension_levels`）

```yaml
language: cpp
qn_separator: "::"
hierarchy:
  - { level: 1, role: namespace, kinds: [NAMESPACE] }
  - { level: 2, role: type,      kinds: [CLASS, STRUCT, ENUM, UNION, TYPE] }
  - { level: 3, role: member,    kinds: [METHOD, METHOD_DECL, FUNCTION, FUNCTION_DECL,
                                          FIELD, ENUM_CONSTANT, VARIABLE,
                                          SCOPE_BLOCK, LAMBDA, MACRO] }
relation_types: [CALL, CREATE, EXTEND, PARAMETER, RETURN, TYPE_ARG,
                 USE, ASSIGN, CAST, THROW, CAPTURE]
structural_relation_types: [CONTAIN, IMPORT, IMPL_LINK]
```

| 项 | C++ 取值 | 说明 |
|---|---|---|
| 层级数 | 3 | namespace → type → member |
| `role=namespace` | `NAMESPACE` | 无 namespace 的全局符号 `namespace_qn` 为空（回退到文件节点） |
| `role=type` | `CLASS` / `STRUCT` / `ENUM` / `UNION` / `TYPE` | `TYPE` 是 typedef 别名，也作为类型节点 |
| `role=member` | 10 类 | **含 `FUNCTION` / `FUNCTION_DECL` / `MACRO`**（C++ 的自由函数与宏没有宿主类型） |
| 结构关系 | `CONTAIN` / `IMPORT` / **`IMPL_LINK`** | `IMPL_LINK` 是 C++ 特有的「声明→定义」链接，**绝不能进依赖边** |
| 依赖关系 | 11 类 | 见 §2 |

> `qn_separator = "::"` 只用于展示；建树一律用 `namespace_qn` / `type_qn`。**禁止**用 `.` 切分 C++ QN。

---

## 2. `relations.type` 取值（C++ 全集，14 种）

实测 `collector/cpp` 样本（739 元素 / 1,145 关系）：

| `type` | 条数 | 源 kind（Top） | 目标 kind（Top） | 有 `Location` | 进入依赖边 |
|---|---|---|---|---|---|
| `CONTAIN` | 819 | FILE 266 / FUNCTION 133 / STRUCT 102 | VARIABLE 260 / FIELD 92 / STRUCT 81 | ❌ | ❌ 结构 |
| `USE` | 102 | FUNCTION 50 / LAMBDA 31 / METHOD 18 | VARIABLE 93 / FIELD 9 | ✅ | ✅ |
| `ASSIGN` | 60 | FUNCTION 26 / SCOPE_BLOCK 16 / METHOD 12 | VARIABLE 46 / FIELD 14 | ✅ | ✅ |
| `TYPE_ARG` | 43 | VARIABLE 43 | CLASS 20 / STRUCT 15 / TYPE 5 | ❌ | ✅ |
| `PARAMETER` | 31 | METHOD_DECL 15 / METHOD 9 / FUNCTION 4 | CLASS 23 / STRUCT 7 | ❌ | ✅ |
| `IMPL_LINK` | 22 | METHOD 15 / FUNCTION 4 | METHOD 15 / FUNCTION 4 | ❌ | ❌ **结构** |
| `RETURN` | 21 | METHOD_DECL 15 / METHOD 5 | CLASS 18 / STRUCT 3 | ❌ | ✅ |
| `CALL` | 15 | FUNCTION 14 / LAMBDA 1 | LAMBDA 13 / METHOD 1 / FUNCTION 1 | ✅ | ✅ |
| `EXTEND` | 13 | STRUCT 13 | STRUCT 13 | ❌ | ✅ |
| `CAPTURE` | 10 | LAMBDA 10 | VARIABLE 10 | ✅ | ✅ |
| `IMPORT` | 4 | FILE 4 | FILE 4 | ✅ | ❌ 结构 |
| `CREATE` | 7 | — | — | ✅ | ✅ |
| `CAST` | 14 | — | — | ✅ | ✅ |
| `THROW` | 6 | — | — | ✅ | ✅ |

**C++ 不产出**：`ANNOTATION`、`IMPLEMENT`、`OVERRIDE`、`MIXIN`、`EXPORT`。
（`IMPLEMENT` 在 C++ 语义上等价于 `EXTEND`，analyzer 统一用 `EXTEND`。）

### 2.1 `IMPL_LINK` 的两端

`IMPL_LINK` 是**唯一两端 QN 完全相同**的关系类型，只能靠 `Kind` 区分：

| `type` | `source_qn` | `source_kind` | `target_qn` | `target_kind` |
|---|---|---|---|---|
| `IMPL_LINK` | `DeclaredClass::declared_method()` | `METHOD` | `DeclaredClass::declared_method()` | `METHOD_DECL` |
| `IMPL_LINK` | `header_func(int)` | `FUNCTION` | `header_func(int)` | `FUNCTION_DECL` |

> 若 `relations` 不存端点 kind，这三类关系永远无法 JOIN 到正确元素。这也是 `elements` 主键必须是 `(qn, kind)` 的直接原因。

---

## 3. `type_edge` 计算口径

### 3.1 `edge_count`

```
输入：relations
① 排除结构关系：type NOT IN ('CONTAIN','IMPORT','IMPL_LINK')   -- 注意 C++ 多一种
② 排除文件/命名空间/包端点：s.kind NOT IN ('FILE','NAMESPACE','PACKAGE')
                          AND t.kind NOT IN ('FILE','NAMESPACE','PACKAGE')
③ 排除自环：s.type_qn <> t.type_qn
④ 端点归一：source_qn = s.type_qn, target_qn = t.type_qn
⑤ 聚合：GROUP BY (source_qn, target_qn)
   edge_count = COUNT(*)
```

实测 `collector/cpp`：**14 对 / 14 条**（样本小，主要是继承边 `Single → BaseA`、`Diamond → VLeft` 等）。

### 3.2 `site_count`

```
有 Location.StartLine > 0  → key = path || ':' || start_line
无 Location                → key = 'NOLOC:' || source_qn
site_count = COUNT(DISTINCT key)
```

C++ 的无行号关系占比更高（`PARAMETER` 31/31、`RETURN` 21/21、`TYPE_ARG` 43/43 全无行号）。

### 3.3 `cross_pkg`

```
cross_pkg = COALESCE(NULLIF(s.namespace_qn,''),'(global)')
         <> COALESCE(NULLIF(t.namespace_qn,''),'(global)')
```

> C++ 用 `(global)` 而非 `(default)`：全局作用域符号的 `namespace_qn` 为空是**正常语义**（不是缺陷），因此这里只是展示兜底，不需要抛给 analyzer。

---

## 4. `call_site` 与 `method_edge`

### 4.1 `call_site`

- 粒度同 Java：`(source_qn, source_kind, target_qn, target_kind, type, path, line)`
- **端点必须带 kind**：`IMPL_LINK` 与 decl/def 共享 QN 的成员，仅按 QN JOIN 会命中多行

### 4.2 `method_edge`（VIEW）的 C++ 白名单

```sql
source_kind IN ('METHOD','METHOD_DECL','FUNCTION','FUNCTION_DECL',
                'LAMBDA','FIELD','ENUM_CONSTANT','SCOPE_BLOCK','MACRO')
AND target_kind IN (同上)
```

| 与 Java 的差异 | 原因 |
|---|---|
| 增加 `FUNCTION` / `FUNCTION_DECL` | C++ 自由函数没有宿主类型，但仍是调用链节点 |
| 增加 `MACRO` | 宏展开是 C++ 的真实调用边 |
| 增加 `METHOD_DECL` | C++ 头文件声明与实现分离，声明也参与调用解析 |

实测 `collector/cpp`：**35 对**（样本小）。

> **`IMPL_LINK` 已被 `structural_relation_types` 排除**，不会进 `call_site`，因此不会污染 `method_edge`。

---

## 5. `type_fan` / `violation_hit`

### 5.1 `type_fan` 的行与列

- 一行 = 一个**宿主类型**（`type_qn`），`qn` 存类型 QN（如 `demo::A`）
- 类型 kind 集合：`CLASS` / `STRUCT` / `ENUM` / `UNION` / `TYPE`
- 孤立类型同样落全 0 行

### 5.2 加权值

算法与 Java 完全一致（§5.2 of `java.md`）：

```
fan_out(q) = Σ edge_count where source_qn = q
fan_in(q)  = Σ edge_count where target_qn = q
```

### 5.3 `violation_hit`

同 Java：一行 = 一个类型的一个违规维度（`fanin` / `fanout`）。

> **C++ 的阈值需单独评估**：C++ 项目普遍存在「头文件声明 + 实现文件定义」的双份元素，`IMPL_LINK` 已排除，但同一逻辑方法仍可能出现 `METHOD` 与 `METHOD_DECL` 两个节点。若阈值按 Java 的 50/150 直接套用，可能低估扇入。建议 C++ 维度配置单独给阈值（首版可沿用 50/150，观察后再调）。

---

## 6. 已知缺陷（待 analyzer 修复，后端不修补）

| # | 缺陷 | 实测 | 期望 |
|---|---|---|---|
| C1 | **`namespace` 重开导致 `TypeQN` 悬空** | `ns_reopen$2::Second` 的 `TypeQN` 指向 `ns_reopen::Second`，但该 QN 的元素实际是 `ns_reopen$2::Second` | analyzer 修正重开 namespace 的 QN 生成规则，保证 `TypeQN` 能命中元素 |
| C2 | 跨文件同名 `static` 被丢弃 | 同 Java J2 | analyzer 修复后主键升级为 `(qn, kind, path)` |
| C3 | `FUNCTION` / `MACRO` / `TYPE` / 全局 `VARIABLE` 无 `type_qn` | 正常语义（无宿主类型），非缺陷 | 走 `element_node` 回退链 `type_qn → namespace_qn → 文件节点` |

> C1 是真实缺陷：后端在 `element_node` 建树时会因 `TypeQN` 落空而把该成员挂到 namespace，**只计数告警，不改写**。

---

## 7. 循环依赖计算（`cycle_group` / `cycle_member` / `cycle_edge`）

### 7.1 算法（与 Java 相同的骨架，差异在输入）

```
输入：type_edge（已按 §3.1 归一化，IMPL_LINK 已排除）
① 按 violation_config.cycle_relation_types 过滤边（C++ 默认 = 11 类型依赖关系）
② 建有向邻接：source_qn → target_qn
③ Tarjan 求强连通分量（SCC）
④ 过滤：
   - 成员数 < 2 丢弃
   - COUNT(DISTINCT namespace_qn) < 2 丢弃
⑤ 落表：cycle_group / cycle_member / cycle_edge
```

### 7.2 C++ 特有注意点

| 注意点 | 说明 |
|---|---|
| **`IMPL_LINK` 必须排除** | 否则 `FUNCTION → FUNCTION_DECL` 会形成「自身到自身」的假环（两端 QN 相同） |
| **`TYPE`（typedef）参与环** | typedef 别名可能形成 `A → B → A`，属真实依赖，保留 |
| **头文件双向 include** | `a.h` include `b.h` 且反之，若 `IMPORT` 未排除会形成假环；`IMPORT` 已在 `structural_relation_types` |
| **`namespace` 边界** | 环判定用 `namespace_qn`（最近外层 namespace），不是文件目录 |

### 7.3 实测样本

`/tmp/cpp_cycle2`（两个类型互相调用）：

```
CALL demo::A::f() -> demo::B::g()   | type_qn: demo::A => demo::B
CALL demo::B::g() -> demo::A::f()   | type_qn: demo::B => demo::A
```

→ SCC = `{demo::A, demo::B}`，若两者同 namespace 则按规则丢弃（包内环不算）；跨 namespace 才保留。

---

## 8. 验收基线（C++，`collector/cpp` 样本）

| 指标 | 值 |
|---|---|
| 元素 / 关系 | 739 / 1,145 |
| 文件 / namespace / 类型元素 | 27 / 23 / 74（`type_qn` 去重） |
| `type_qn` 悬空 | 1（C1，待 analyzer 修） |
| `type_edge` | 14 对 / 14 条 |
| `method_edge` | 35 对 |
| 关系类型 | 14 种（含 `IMPL_LINK`） |
| 无行号关系 | `PARAMETER`/`RETURN`/`TYPE_ARG`/`EXTEND`/`IMPL_LINK`/`CONTAIN` 全部无行号 |
