# Java 语言口径

> 本文定义 Java 在**原始层 → 维度层 → 预计算层**的语言特化规则。通用规则见 [development.md](../development.md)，表结构见 [schema.md](../schema.md)。
> 数据基线：analyzer `76745eb`，`cleanarch-backend` 933 文件 / 35,706 元素 / 100,321 关系（2026-11-17）。

---

## 1. 维度配置（`dimensions` / `dimension_levels`）

```yaml
language: java
qn_separator: "."
hierarchy:
  - { level: 1, role: namespace, kinds: [PACKAGE] }
  - { level: 2, role: type,      kinds: [CLASS, INTERFACE, ENUM, ANNOTATION] }
  - { level: 3, role: member,    kinds: [METHOD, FIELD, ENUM_CONSTANT, VARIABLE,
                                          SCOPE_BLOCK, LAMBDA, METHOD_REF, ANONYMOUS_CLASS] }
relation_types: [CALL, CREATE, EXTEND, IMPLEMENT, TYPE_ARG, CAPTURE,
                 USE, ASSIGN, CAST, PARAMETER, RETURN, THROW, ANNOTATION]
structural_relation_types: [CONTAIN, IMPORT]
```

| 项 | Java 取值 | 说明 |
|---|---|---|
| 层级数 | 3 | package → type → member |
| `role=namespace` | `PACKAGE` | 253 个；含默认包哨兵 `(default)`（见 §6） |
| `role=type` | `CLASS` / `INTERFACE` / `ENUM` / `ANNOTATION` | 1,059 个元素；去重宿主 `type_qn` 934 个（嵌套类归并） |
| `role=member` | 8 类 | 含 `ANONYMOUS_CLASS`（36 个）与 `METHOD_REF`（423 个），旧配置漏配 |
| 结构关系 | `CONTAIN` / `IMPORT` | 不参与依赖边 |
| 依赖关系 | 13 类 | 见 §2 |

> `qn_separator = "."` 只用于**展示与兜底**；建树一律用 `namespace_qn` / `type_qn`，不切字符串。

---

## 2. `relations.type` 取值（Java 全集，15 种）

实测 100,321 条关系按类型与端点 kind 分布（下表按 `00df5f3` 的 100,312 条统计；默认包修复后仅 `CONTAIN` +9，即 `(default) → 9 个默认包文件`，其余类型不变）：

| `type` | 条数 | 源 kind（Top） | 目标 kind（Top） | 有 `Location` | 进入依赖边 |
|---|---|---|---|---|---|
| `CONTAIN` | 37,558 | CLASS 14,770 / METHOD 11,721 / PACKAGE 2,100 | VARIABLE 11,231 / METHOD 11,157 / FIELD 4,812 | ❌ | ❌ 结构 |
| `USE` | 31,477 | METHOD 28,071 / LAMBDA 2,747 / ENUM_CONSTANT 253 | VARIABLE 21,954 / FIELD 7,734 / ENUM_CONSTANT 1,415 | ✅ | ✅ |
| `CALL` | 11,159 | METHOD 10,468 / LAMBDA 512 / FIELD 130 | METHOD 11,157 / VARIABLE 2 | ✅ | ✅ |
| `ASSIGN` | 7,665 | METHOD 6,194 / FIELD 1,020 / LAMBDA 451 | VARIABLE 5,593 / FIELD 2,072 | ✅ | ✅ |
| `IMPORT` | 3,291 | FILE 3,291 | CLASS 2,568 / ENUM 457 / INTERFACE 166 | ✅ | ❌ 结构 |
| `RETURN` | 2,275 | METHOD 2,275 | CLASS 1,855 / ENUM 416 | ❌ | ✅ |
| `TYPE_ARG` | 2,248 | METHOD 1,018 / VARIABLE 894 / FIELD 172 | CLASS 1,883 / ENUM 329 | ❌ | ✅ |
| `PARAMETER` | 1,606 | METHOD 1,606 | CLASS 1,345 / ENUM 155 | ❌ | ✅ |
| `CREATE` | 1,459 | METHOD 1,314 / LAMBDA 84 | CLASS 1,457 / ENUM 2 | ✅ | ✅ |
| `THROW` | 651 | METHOD 647 / LAMBDA 4 | CLASS 651 | ✅ 630 / ❌ 21 | ✅ |
| `CAPTURE` | 682 | LAMBDA 660 / SCOPE_BLOCK 22 | VARIABLE 513 / FIELD 169 | ✅ | ✅ |
| `ANNOTATION` | 81 | FIELD 37 / METHOD 30 / CLASS 13 | ANNOTATION 68 / CLASS 13 | ❌ | ✅ |
| `EXTEND` | 59 | CLASS 52 / INTERFACE 7 | CLASS 52 / INTERFACE 7 | ❌ | ✅ |
| `CAST` | 54 | METHOD 54 | CLASS 54 | ✅ | ✅ |
| `IMPLEMENT` | 47 | CLASS 47 | INTERFACE 47 | ❌ | ✅ |

**Java 不产出**：`IMPL_LINK`（无声明/实现分离）、`MIXIN`、`OVERRIDE`、`EXPORT`。

> `CALL` 的目标 `VARIABLE` 仅 2 条（`X::new` 方法引用形态），正常应为 `METHOD`。
> `THROW` 有 21 条无 `Location`（`throws` 声明子句），其余 630 条是 `throw new X()` 语句。

---

## 3. `type_edge` 计算口径

### 3.1 `edge_count`

```
输入：relations
① 排除结构关系：type NOT IN ('CONTAIN','IMPORT')
② 排除文件/包端点：s.kind NOT IN ('FILE','PACKAGE') AND t.kind NOT IN ('FILE','PACKAGE')
③ 排除自环：s.type_qn <> t.type_qn        -- 类型内引用不计
④ 端点归一：source_qn = s.type_qn, target_qn = t.type_qn   -- 嵌套类归并到宿主
⑤ 聚合：GROUP BY (source_qn, target_qn)
   edge_count = COUNT(*)                  -- 加权：一行 relations = 一次出现
```

实测：**3,549 对 / 18,022 条**（自环过滤掉约 41,000 条类型内引用）。

### 3.2 `site_count`

```
有 Location.StartLine > 0  → key = path || ':' || start_line
无 Location（6 类关系）    → key = 'NOLOC:' || source_qn
site_count = COUNT(DISTINCT key)
```

实测全库 **15,269** 个引用点。若只用 `path:line`，无行号的 `PARAMETER`/`RETURN`/`TYPE_ARG`/`EXTEND`/`IMPLEMENT`/`ANNOTATION` 会被压成 1 点（ServiceException 582 → 654）。

### 3.3 `cross_pkg`

```
cross_pkg = COALESCE(NULLIF(s.namespace_qn,''),'(default)')
         <> COALESCE(NULLIF(t.namespace_qn,''),'(default)')
```

实测 **3,203 / 3,549** 对跨包。analyzer 现已为默认包元素直出 `namespace_qn = "(default)"`，`COALESCE` 仅用于兼容旧产物与 C++ 等「空 `namespace_qn` 是正常语义」的语言；去掉它会让旧产物产生约 40 个假跨包对（§6）。

### 3.4 点开类型边的成员明细

```sql
SELECT cs.type, cs.path, cs.line, cs.call_count,
       cs.source_qn AS source_member, cs.source_kind,
       cs.target_qn AS target_member, cs.target_kind
FROM call_site cs
JOIN elements s ON s.qn = cs.source_qn AND s.kind = cs.source_kind
JOIN elements t ON t.qn = cs.target_qn AND t.kind = cs.target_kind
WHERE s.type_qn = ? AND t.type_qn = ?
ORDER BY cs.path, cs.line;
```

实测 2,832 / 3,549 对（80%）有调用点；剩余 **717 对是纯类型引用**（TYPE_ARG 366 / PARAMETER 232 / RETURN 118 / IMPLEMENT 44 / EXTEND 41 / ANNOTATION 31 / THROW 2），UI 要显示「仅类型引用」而非空列表。

---

## 4. `call_site` 与 `method_edge`

### 4.1 `call_site`

- 粒度：`(source_qn, source_kind, target_qn, target_kind, type, path, line)`
- 实测 **58,083 行 / 59,463 条**，其中 **6,337 行无行号**（`path=''` / `line=0`）
- 有行号的 51,780 行 / 53,126 条

### 4.2 `method_edge`（VIEW）的 Java 白名单

```sql
source_kind IN ('METHOD','FIELD','ENUM_CONSTANT','LAMBDA','METHOD_REF','SCOPE_BLOCK')
AND target_kind IN ('METHOD','FIELD','ENUM_CONSTANT','LAMBDA','METHOD_REF','SCOPE_BLOCK')
```

实测 **17,314 对 / 22,530 条**（0 个无调用点）。

| 被排除的关系（带行号） | 条数 | 理由 |
|---|---|---|
| `METHOD→VARIABLE (USE)` | 19,532 | 类型内局部变量，对 Blast Radius 无意义 |
| `METHOD→VARIABLE (ASSIGN)` | 5,143 | 同上 |
| `LAMBDA→VARIABLE (USE)` | 2,403 | 同上 |
| `LAMBDA→VARIABLE (CAPTURE)` | 511 | 同上 |
| `LAMBDA→VARIABLE (ASSIGN)` | 450 | 同上 |
| `METHOD→CLASS (USE)` | 97 | 静态引用，非调用 |

> **不放行 `CLASS` 目标**：Java 的构造器调用在 analyzer `633c58f` 后已直接指向 `METHOD`（真实构造器元素），无需额外放行 `CLASS`。这与 11-11 旧产物不同（当时 1,460 条构造器端点悬空）。

---

## 5. `type_fan` / `violation_hit`

### 5.1 `type_fan` 的行与列

- 一行 = 一个**宿主类型**（`type_qn`），`qn` 存类型 QN（如 `com.x.entity.Task`）
- 共 **934 行**（934 个宿主类型），其中 912 个有边、**22 个孤立类型落全 0 行**
- 列：`fan_in` / `fan_out`（加权）、`fan_in_types` / `fan_out_types`（去重类型数）、`fan_in_sites` / `fan_out_sites`（引用点数）

### 5.2 加权值怎么算

```
fan_out(q) = Σ edge_count  where source_qn = q     -- 该类型「引用出去」的总次数
fan_in(q)  = Σ edge_count  where target_qn = q     -- 该类型「被引用」的总次数
fan_out_types(q) = COUNT(DISTINCT target_qn)
fan_in_types(q)  = COUNT(DISTINCT source_qn)
fan_out_sites(q)   = Σ site_count
fan_in_sites(q)    = Σ site_count
```

实测 Top3（加权 / 引用点 / 依赖类型数）：

| 类 | `fan_in` | `fan_in_sites` | `fan_in_types` |
|---|---|---|---|
| `ServiceException` | 1,697 | 582 | 139 |
| `Task` | 680 | 577 | 67 |
| `Constant` | 471 | 408 | 147 |

最大扇出 420（加权）/ 375（引用点）。

### 5.3 `violation_hit`

**是**「按当前 `violation_config` 命中的违规类型清单」，一行 = 一个类型的一个违规维度（`kind ∈ fanin | fanout`），同一类型可两行。

- 阈值默认 `fan_warn=50` / `fan_severe=150`
- `dedupe_same_site=true` 时用 `fan_*_sites` 比较，`false` 时用加权 `fan_*`
- 实测：加权口径命中 79 / 13（扇入）、99 / 21（扇出）；去重口径 72 / 11、84 / 14

---

## 6. 已知缺陷（待 analyzer 修复，后端不修补）

| # | 缺陷 | 实测 | 期望 |
|---|---|---|---|
| J1 | ~~**默认包无归属**~~ **已修复** | 9 个测试文件（`OperationDataTest` 等）无 `package` 声明 → analyzer 现直出 `NamespaceQN = "(default)"` 并注册 `(default)` PACKAGE 元素，295 元素全部有归属 | 已由 analyzer 修复；后端 `COALESCE` 兜底仅保留给旧产物与 C++ |
| J2 | 跨文件同名 `static` 被丢弃 | 两文件各写 `static int helper(int)` → 第二个元素消失，调用被错误解析到第一个 | analyzer 修复后主键可升级为 `(qn, kind, path)` |
| J3 | 文本块污染 `NamespaceQN` | Java 15 `"""..."""` 作为调用接收者时，目标 QN 与 `NamespaceQN` 取整段字面量 | analyzer 侧过滤 |

> 后端**不得**为上述缺陷做 QN/kind 改写（见 [development.md](../development.md) §2.3）。

---

## 7. 循环依赖计算（`cycle_group` / `cycle_member` / `cycle_edge`）

### 7.1 算法

```
输入：type_edge（已按 §3.1 归一化）
① 按 violation_config.cycle_relation_types 过滤边（默认 = 全部 13 类型依赖关系）
② 建有向邻接：source_qn → target_qn
③ Tarjan 求强连通分量（SCC）
④ 过滤：
   - 成员数 < 2 丢弃
   - COUNT(DISTINCT namespace_qn) < 2 丢弃（包内环不算，用户 11.16 拍板）
⑤ 落表：
   cycle_group  (group_id, member_count, pkg_count)
   cycle_member (group_id, type_qn, namespace_qn)
   cycle_edge   (group_id, source_qn, target_qn, types, site_count)
```

### 7.2 Java 实测结果

| SCC | 成员 | 包数 | 结果 |
|---|---|---|---|
| 1 | `EnumValidator` ↔ `EnumValid` | 1 | **丢弃**（包内环） |
| 2 | `ProjectUtils` ↔ `Project` | 2 | **保留**（唯一跨包环） |
| 3 | `UuidsValid` ↔ `UuidsValidator` | 1 | **丢弃**（包内环） |

`cycle_edge` 明细：

| source | target | types | site_count | 引用点 |
|---|---|---|---|---|
| `...entity.Project` | `...utils.ProjectUtils` | `[CALL]` | 1 | `Project.java:289` |
| `...utils.ProjectUtils` | `...entity.Project` | `[CALL, PARAMETER]` | 3 | `ProjectUtils.java:108`、`:111`、1 条无行号 PARAMETER |

### 7.3 为什么必须先在类型级归一化

旧实现直接在 `level_edges`（含 CONTAIN 背景边、未归并嵌套类）上跑 Tarjan，得到 **18 组假环**。两个根因：

1. **嵌套类未归并**：`LoginVo.Builder ↔ LoginVo` 被当成两个类型互调；
2. **包内环未过滤**：validator 注解 ↔ 校验器这类同包互引不构成架构环。

---

## 8. 验收基线（Java）

| 指标 | 期望值 |
|---|---|
| 元素 / 关系 | 35,706 / 100,321 |
| 包 / 类型元素 / 宿主类型 | 253 / 1,059 / 934 |
| `type_edge` | 3,549 对 / 18,022 条 / 15,269 引用点（跨包 3,203） |
| `call_site` | 58,083 行（6,337 行无行号） |
| `method_edge` | 17,314 对 / 22,530 条 |
| `type_fan` | 934 行（22 个孤立类型） |
| 跨包环组 | 1 |
| 高扇入（加权 ≥50 / ≥150） | 79 / 13 |
| 高扇入（去重 ≥50 / ≥150） | 72 / 11 |
| 最大扇入 | 1,697（`ServiceException`）/ 582（去重） |
