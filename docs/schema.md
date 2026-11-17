# 表结构详解与真实案例数据

> 表结构的设计理由见 [development.md](development.md) §2.8。本文只做两件事：**逐表解释设计意图**、**用同一个真实案例把每张表的数据列出来**。
>
> 数据来源：analyzer `76745eb` 对 `cleanarch-backend` 的**新一次重跑**（933 文件 / 35,706 元素 / 100,321 关系）。相对 `00df5f3` 的增量仅为默认包：+1 `(default)` PACKAGE 元素与 +9 条 `(default) → 文件` CONTAIN 关系。`.arch_lens/sources/7af6d6e8b785/output` 仍是 11-11 旧产物，需重新 analyze + ingest 才能与本文对齐。

---

## 0. 案例源码

选 `com.huawei.tenet.common.utils.AviatorUtil`（真实数据，源码 23–40 行），它同时覆盖：嵌套作用域块、Lombok 生成字段、隐式默认构造器、跨类调用、跨包调用。

```java
@Slf4j
public class AviatorUtil {                                              // 23
    public static List<String> getVariables(@NonNull String expression) {  // 31
        try {
            Expression compile = AviatorEvaluator.compile(expression);  // 33
            return compile.getVariableNames();                          // 34
        } catch (ExpressionRuntimeException e) {                        // 35
            log.error(MessageUtils.getMessage(e));                      // 36
            return Collections.emptyList();
        }
    }
}                                                                       // 40
```

---

## 1. 分层与表清单

```
① 原始层（ingest 写入，全量重建）
     elements / relations
② 维度层（配置驱动，建树结果）
     dimensions / dimension_levels / hierarchy_node / element_node
③ 预计算层（对齐前端交互）
     type_edge / call_site / method_edge(VIEW) / type_fan
     violation_config / violation_hit
     cycle_group / cycle_member / cycle_edge
     pkg_rollup / metrics
```

设计原则：

1. **① 只做字段保真**，不做语义解析；
2. **② 只描述「树长什么样」**，不关心具体元素归属；
3. **③ 只放「前端一次查询就要的结构」**，不在请求期现算。

任何一层都不做跨层推导。

---

## 2. 原始层

### 2.1 `elements`

```sql
CREATE TABLE elements (
    qn             VARCHAR NOT NULL,
    kind           VARCHAR NOT NULL,
    name           VARCHAR,
    path           VARCHAR,
    start_line     INTEGER,
    end_line       INTEGER,
    signature      VARCHAR,
    doc            VARCHAR,   -- JSONL Doc
    comment        VARCHAR,   -- JSONL Comment
    namespace_qn   VARCHAR,   -- JSONL NamespaceQN
    enclosing_qn   VARCHAR,   -- JSONL EnclosingQN
    type_qn        VARCHAR,   -- JSONL TypeQN
    is_source      BOOLEAN,
    is_sugar       BOOLEAN,   -- 编译器隐式生成
    is_impl_detail BOOLEAN,   -- 源码语法结构（LAMBDA/ANON/MREF/BLOCK）
    extra          JSON,      -- JSONL Extra：Modifiers / Annotation / Mores
    PRIMARY KEY (qn, kind)
);
```

| 设计点 | 说明 |
|---|---|
| **主键 `(qn, kind)`** | C++ 声明/定义共享 QN（e2e 样本 1,417 元素中 76 个重复，72 个 decl+def 对），`qn` 单列不唯一。Java 实测 `(qn,kind)` 0 重复 |
| `extra JSON` | 35,443 条 / 6.41 MB（平均 188 B）；含 `java.method.is_constructor`(11,131)、`java.variable.raw_type`(11,226)、`java.method.return_type`(7,528)、`java.method.parameters`(6,439)、`java.lombok.annotation_type`(7,518) |
| `doc` / `comment` | 3,271 / 1,264 条，共 0.28 MB；L3 源码面板用 |
| 三个归属列 | 全部来自 JSONL 原样写入。**唯一例外**：`namespace_qn` 为空时补 `(default)`（G17），仅用于兼容旧产物与 C++ 等「空 `namespace_qn` 是正常语义」的语言；Java 默认包已由 analyzer 直出 `(default)` |
| `is_sugar` vs `is_impl_detail` | 前者 = 编译器隐式生成（Lombok/默认构造器）；后者 = 源码语法结构（LAMBDA/ANON/MREF/BLOCK）。**两者正交**，不能合并 |
| `path` | 相对路径，与 `manifest.root_path` 拼绝对路径读盘（源码元素树也直接查本表） |

### 2.2 `relations`

```sql
CREATE TABLE relations (
    source_qn   VARCHAR NOT NULL,
    source_kind VARCHAR NOT NULL,   -- JSONL Source.Kind
    target_qn   VARCHAR NOT NULL,
    target_kind VARCHAR NOT NULL,   -- JSONL Target.Kind
    type        VARCHAR,
    path        VARCHAR,
    start_line  INTEGER,
    end_line    INTEGER,
    raw_text    VARCHAR
);
```

| 设计点 | 说明 |
|---|---|
| **端点 kind 必须存** | C++ `IMPL_LINK` 两端 QN 完全相同（`FUNCTION` → `FUNCTION_DECL`），只能靠 Kind 区分。实测 Java 100,321 条关系两端 100% 带 `Kind`，无需 analyzer 改动 |
| 无主键 | 同一 `(src,tgt,type,line)` 可重复（实测 1,887 组），去重是**预计算层的事**，原始层保真 |
| `path` 必须存 | 旧表只存 `start_line`，导致调用点明细无法还原（G1）。实测 63,080 条有 `path`+`line` |
| `raw_text` 存 `Mores` JSON | 调用点展开时展示 `AviatorUtil.getVariables(rule)` 这类原文 |
| **端点由 analyzer 保证可解析** | `633c58f` 后 18,490 个 `xxx()` 端点全部命中 `elements`，外部端点 0 条；`relations` 与 `elements` 可干净 JOIN，后端不做改写 |

---

## 3. 维度层

### 3.1 `dimensions` + `dimension_levels`

```sql
CREATE TABLE dimensions (
    id           INTEGER PRIMARY KEY,
    language     VARCHAR NOT NULL,
    qn_separator VARCHAR DEFAULT '.',
    imported_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE (language)
);

CREATE TABLE dimension_levels (
    dimension_id INTEGER,
    level        INTEGER,
    role         VARCHAR NOT NULL,   -- namespace | type | member
    kinds        VARCHAR NOT NULL,   -- 本层接受的 ElementKind 白名单（逗号串）
    UNIQUE (dimension_id, level),
    UNIQUE (dimension_id, role)
);
```

| 设计点 | 说明 |
|---|---|
| `UNIQUE(language)` | 每个库每种语言唯一；避免重复 register 产生孤儿行（`DimensionID()` 只取第一条） |
| 去 `name` 列 | 原 `InsertDimension(id, name, language)` 两个参数传同值，纯冗余 |
| `qn_separator` | 只用于**包链拆分和前端短名**，不参与归属判定（归属靠三字段） |
| `role` | 查询一律用 `role`，不用 `level` 数字（G16） |
| **`kinds` 合并了原 `kind` + `parent_of`** | 原 `kind`（该层代表 kind）与 `parent_of`（本层接受的 kind 白名单）语义重叠；且 `kind: MEMBER` 不是真实 `ElementKind`，`kindSet()` 塞进白名单是空操作。合并后语义单一 |

Java 配置（已补 7 类 kind，覆盖此前遗漏的 17,492 个元素）：

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

### 3.2 `hierarchy_node`

```sql
CREATE TABLE hierarchy_node (
    dimension_id INTEGER, qn VARCHAR, level INTEGER,
    role         VARCHAR,
    parent_qn    VARCHAR,
    depth        INTEGER,
    kind         VARCHAR,
    PRIMARY KEY (dimension_id, qn, kind)
);
```

| 设计点 | 说明 |
|---|---|
| 只放**容器节点** | 包 + 类型。成员不建节点（成员没有子节点，建了浪费） |
| `parent_qn` 是树边 | 包→包、类型→包。**实测 `enclosing_qn` 与 CONTAIN 父节点 33,576 条 0 mismatch**，可直接用归属字段建 |
| `depth` 新增 | P9 包上卷按 `depth` 降序逐级累加；原 DDL 缺此列 |
| 主键 `(dimension_id,qn,kind)` | C++ 的 `NAMESPACE` 与 `TYPE` 可能同名 |

> **包链仍需要 `qn_separator`**：`namespace_qn` 只给到元素所属的**最内层**命名空间，父包链（`com.huawei.tenet.common.utils` → `...common` → `...tenet` → `...huawei` → `com`）必须从 QN 拆分或从 `CONTAIN` 关系推导。Java 实测 251 条 `PACKAGE→PACKAGE` CONTAIN 边，可直接用；C++ 的 `NAMESPACE→NAMESPACE` 同理。**优先用 CONTAIN，`qn_separator` 只做兜底**。

### 3.3 `element_node`

```sql
CREATE TABLE element_node (
    element_qn   VARCHAR,
    element_kind VARCHAR,      -- 新增
    dimension_id INTEGER,
    node_qn      VARCHAR,
    level        INTEGER,
    PRIMARY KEY (dimension_id, element_qn, element_kind)
);
```

| 设计点 | 说明 |
|---|---|
| 类型自身也有一行 | `(AviatorUtil, CLASS, 1, AviatorUtil, 2)`，否则 `ChildNodes` 查不到类型 |
| **挂载回退链** | `node_qn = type_qn` → 若空则 `namespace_qn` → 若空则文件节点 |
| 为什么需要回退 | C++ 的 `FUNCTION`/`FUNCTION_DECL`/`MACRO`/`TYPE`/全局 `VARIABLE`/`NAMESPACE` 的 `type_qn` 为空；Java 默认包元素 `namespace_qn` 已由 analyzer 直出 `(default)`，回退链只用于旧产物 |
| 成员指向**宿主类型** | 有 `type_qn` 时用它，不是 `enclosing_qn`。lambda 的 `enclosing_qn` 是块，但归属仍是宿主类型 |
| `element_kind` 新增 | C++ decl/def 共享 QN，只按 `element_qn` 关联会产生歧义 |

---

## 4. 预计算层

### 4.1 `type_edge`

```sql
CREATE TABLE type_edge (
    dimension_id INTEGER,
    source_qn VARCHAR, target_qn VARCHAR,
    types VARCHAR[],          -- 原生数组
    edge_count INTEGER,       -- COUNT(*)，加权
    site_count INTEGER,       -- 去重引用点数（有行号按 path:line，无行号按 source_qn）
    cross_pkg BOOLEAN,
    PRIMARY KEY (dimension_id, source_qn, target_qn)
);
```

| 设计点 | 说明 |
|---|---|
| `source_qn/target_qn` 是 `type_qn` | 嵌套类已归并到宿主类型 |
| `edge_count` 保留**加权值** | 边详情要展示「调了多少次」，不能去重 |
| `site_count` 是阈值口径 | `type_fan.fan_*_sites` 直接取它，不必回扫原始 `relations`（P7）。去重键 = 有行号用 `path:line`，无行号用 `source_qn`（否则 6 种无行号关系会被压成 1 点，582 → 654） |
| `types` 用原生数组 | 支持 `list_has_any(types, ['CALL'])` 下推（G7）。但 `duckdbthin` 读回是 `"[call, use]"` 字符串，repository 要解析 |
| `cross_pkg` | 建表时算好，避免每次 JOIN `elements` 比 `namespace_qn`；**必须在默认包兜底（P2）之后算**，否则旧产物的空 `namespace_qn` 会产生约 40 个假跨包对 |
| 主键 | 防重跑重复插入；重跑前仍建议 `DELETE WHERE dimension_id=?` |

**点开类型边后，成员调用详情怎么查**（不能直接查 `type_edge`，它已丢弃成员与行号）：

```sql
SELECT cs.type, cs.path, cs.line, cs.call_count,
       cs.source_qn AS source_member, cs.source_kind,
       cs.target_qn AS target_member, cs.target_kind
FROM call_site cs
JOIN elements s ON s.qn = cs.source_qn AND s.kind = cs.source_kind
JOIN elements t ON t.qn = cs.target_qn AND t.kind = cs.target_kind
WHERE s.type_qn = ? AND t.type_qn = ?      -- type_edge 的 source_qn / target_qn
ORDER BY cs.path, cs.line;
```

实测 3,549 对里 2,832 对（80%）能查出明细；剩余 717 对是**纯类型引用**（无行号），UI 要显示「仅类型引用」而不是空列表（P1）。

### 4.2 `call_site`

```sql
CREATE TABLE call_site (
    dimension_id INTEGER,
    source_qn VARCHAR, source_kind VARCHAR,
    target_qn VARCHAR, target_kind VARCHAR,
    type VARCHAR,
    path VARCHAR NOT NULL, line INTEGER NOT NULL,   -- 无行号写 '' / 0
    call_count INTEGER,
    PRIMARY KEY (dimension_id, source_qn, source_kind,
                 target_qn, target_kind, type, path, line)
);
```

| 设计点 | 说明 |
|---|---|
| 粒度 = `(src,src_kind,tgt,tgt_kind,type,path,line)` | 同一位置可能有 ASSIGN + USE 两条（实测 1,630 组），按 type 分开存 |
| 端点带 kind | C++ decl/def 共享 QN，仅按 QN JOIN `elements` 会歧义（D4） |
| 这是「边详情」的唯一数据源 | L1 类型对展开、L2 方法对展开、`method_edge` VIEW 都读它 |
| 只有 8 种关系有位置 | assign/call/capture/cast/create/import/throw/use；parameter/return/type_arg/extend/implement/annotation 无行号（P13：全量 58,083 行，其中 6,337 行无行号，不能丢） |
| 无行号行用哨兵值 | DuckDB 主键隐含 NOT NULL，所以写 `path=''` / `line=0`，查询时用 `line > 0` 判有行号 |

### 4.3 `method_edge`（VIEW）

```sql
CREATE VIEW method_edge AS
SELECT dimension_id, source_qn, target_qn,
       array_agg(DISTINCT type) AS types,
       SUM(call_count)          AS edge_count
FROM call_site
WHERE source_kind IN ('METHOD','FIELD','ENUM_CONSTANT','LAMBDA','METHOD_REF','SCOPE_BLOCK')
  AND target_kind IN ('METHOD','FIELD','ENUM_CONSTANT','LAMBDA','METHOD_REF','SCOPE_BLOCK')
GROUP BY 1,2,3;
```

| 设计点 | 说明 |
|---|---|
| **不落表** | 实测 17,314 对全部可由 `call_site` 推导（0 个无调用点），落表纯冗余（P5） |
| 源端 kind ∈ 固定 6 类 | METHOD/FIELD/ENUM_CONSTANT/LAMBDA/METHOD_REF/SCOPE_BLOCK（G21）。白名单必须写进维度配置，否则口径漂移 12% |
| 两端白名单由维度配置提供 | Java 与 C++ 的 member kind 集合不同（§6.8），VIEW 定义随维度切换 |
| **不放行 `CLASS` 目标** | analyzer `633c58f` 后 `new X()` 的 CALL 目标已指向真实构造器 `METHOD`，无需为构造器额外放行类型 kind（旧产物才需要，会多算 1,107 对） |
| 白名单外丢弃 28,507 条带行号关系 | 最大项 `METHOD→VARIABLE (USE)` 19,532 条、`METHOD→VARIABLE (ASSIGN)` 5,143 条，绝大多数是类型内局部变量，对 Blast Radius 无意义 |

### 4.4 `type_fan`

```sql
CREATE TABLE type_fan (
    dimension_id INTEGER, qn VARCHAR,
    kind VARCHAR,                                     -- 宿主类型 kind（type_qn→kind 实测 1:1）
    fan_in INTEGER, fan_out INTEGER,                  -- 加权 SUM(edge_count)
    fan_in_types INTEGER, fan_out_types INTEGER,  -- 去重依赖类型数
    fan_in_sites INTEGER, fan_out_sites INTEGER,      -- 去重引用点数（阈值判定）
    PRIMARY KEY (dimension_id, qn)
);
```

| 设计点 | 说明 |
|---|---|
| `qn` 是**宿主类型 QN**（`type_qn`），不是成员 | 一行 = 一个类型/接口/枚举/注解；`kind` 存该类型的 ElementKind |
| `kind` 可安全冗余 | 实测 934 个 `type_qn` 每个恰好对应 1 个类型元素（0 歧义），无需回查 `elements` |
| **两套值并存** | 加权值给展示，`sites` 给阈值。实测 ServiceException 加权 1,697 / 去重 582，阈值命中 79 vs 72 |
| 与阈值无关 | 只算一次；改阈值不动这张表 |
| `fan_*_sites` 来自 `type_edge.site_count` | 不需要回扫原始 `relations`（P7） |
| 孤立类型 | 934 个类型中 22 个无跨类型边，**也要落一行全 0**，否则前端类型列表缺项（P6） |

### 4.5 `violation_config`

```sql
CREATE TABLE violation_config (
    dimension_id INTEGER PRIMARY KEY,
    fan_warn INTEGER DEFAULT 50,
    fan_severe INTEGER DEFAULT 150,
    cycle_relation_types VARCHAR[],
    merge_nested BOOLEAN DEFAULT TRUE,
    dedupe_same_site BOOLEAN DEFAULT TRUE,
    updated_at TIMESTAMP DEFAULT current_timestamp
);
```

| 设计点 | 说明 |
|---|---|
| `dimension_id` 做主键 | 一个维度一份口径 |
| `updated_at` 兼作版本号 | 前端据此丢弃过期响应 |
| `dedupe_same_site` | 默认 true。为 false 时阈值比较用 `fan_in`（加权），为 true 用 `fan_in_sites` |

### 4.6 `violation_hit`

```sql
CREATE TABLE violation_hit (
    dimension_id INTEGER, qn VARCHAR,
    metric VARCHAR,      -- fanin | fanout
    severity VARCHAR,    -- warn | severe
    value INTEGER,       -- 命中时用的值（dedupe 时是 fan_*_sites）
    type_count INTEGER,  -- 去重依赖类型数
    PRIMARY KEY (dimension_id, qn, metric)
);
```

| 设计点 | 说明 |
|---|---|
| 只存**命中行** | 未命中的类型不出现，表规模 ≈ 命中数（实测 79+99 行量级） |
| 同一类型可两行 | `metric = fanin / fanout` 各一行 |
| 重算时整表重建 | 先 `DELETE WHERE dimension_id=?` 再插入 |

### 4.7 `cycle_group` + `cycle_member` + `cycle_edge`

> 原 `class_cycle` 命名有歧义：它一行存的是「某个环组里的一个类型」，读起来却像「某个类型上有环」。拆成三张表（P9）。

```sql
CREATE TABLE cycle_group (
    dimension_id INTEGER, group_id INTEGER,
    member_count INTEGER, pkg_count INTEGER,
    PRIMARY KEY (dimension_id, group_id)
);
CREATE TABLE cycle_member (
    dimension_id INTEGER, group_id INTEGER,
    type_qn VARCHAR, namespace_qn VARCHAR,
    PRIMARY KEY (dimension_id, group_id, type_qn)
);
CREATE TABLE cycle_edge (                      -- 原 cycle_hop
    dimension_id INTEGER, group_id INTEGER,
    source_qn VARCHAR, target_qn VARCHAR,      -- 两端都是类型 QN（type_qn）
    types VARCHAR[], site_count INTEGER,
    PRIMARY KEY (dimension_id, group_id, source_qn, target_qn)
);
```

| 设计点 | 说明 |
|---|---|
| `group_id` 是 SCC 序号 | 一个 SCC 的成员在 `cycle_member` 里多行，`group_id` 相同 |
| `namespace_qn` 冗余存 | 前端画环时直接拿包名，不必回查 `elements` |
| `cycle_edge` 只存**组内边** | 用于画布高亮环边 + 链路展示 |
| `cycle_edge` 两端是**类**不是包 | 前端要在画布上高亮具体类型节点（`L1OverviewView.vue:691` 用 hops 建邻接图找最短环链路） |
| `site_count` 而非 `path/line` | `cycle_edge` 是类型对，`call_site` 是成员对，存在 JOIN 断点；先给条数，点开时经 `elements.type_qn` 中转查 `call_site`（P10/P11） |
| 过滤规则在建表时执行 | 成员 <2 丢弃；`COUNT(DISTINCT namespace_qn) < 2` 丢弃（包内环不算） |

**环上某条边的调用点明细怎么查**（`cycle_edge` 是类型对，`call_site` 是成员对，必须经 `elements.type_qn` 中转）：

```sql
SELECT cs.type, cs.path, cs.line, cs.call_count,
       cs.source_qn AS source_member, cs.target_qn AS target_member
FROM call_site cs
JOIN elements s ON s.qn = cs.source_qn AND s.kind = cs.source_kind
JOIN elements t ON t.qn = cs.target_qn AND t.kind = cs.target_kind
WHERE s.type_qn = ? AND t.type_qn = ?      -- cycle_edge 的 source_qn / target_qn
ORDER BY cs.path, cs.line;
```

### 4.8 `pkg_rollup`

```sql
CREATE TABLE pkg_rollup (
    dimension_id INTEGER, qn VARCHAR,
    subtree_type_count INTEGER,
    subtree_cycle_groups INTEGER,
    subtree_high_fanin INTEGER,
    subtree_high_fanout INTEGER,
    direct_type_count INTEGER,
    PRIMARY KEY (dimension_id, qn)
);
```

| 设计点 | 说明 |
|---|---|
| 自底向上累加 | 沿 `parent_qn` 链，每个包存**子树**值 |
| `direct_type_count` 单独存 | 前端要区分「本包直接类型数」和「子树类型数」 |
| 环组按组去重 | 一个环跨多个子包时，共同祖先只计 1 次 |

### 4.9 `metrics`

```sql
CREATE TABLE metrics (
    dimension_id INTEGER, kind VARCHAR, count INTEGER,
    PRIMARY KEY (dimension_id, kind)
);
```

| `kind` | 口径 |
|---|---|
| `element_total` | `COUNT(*) FROM elements` |
| `relation_total` | `COUNT(*) FROM relations` |
| `type_element_total` | 全部类型元素 `kind IN ('CLASS','INTERFACE','ENUM','ANNOTATION')` = 1,059（原实现漏 ANNOTATION，M1） |
| `type_total` | 去重宿主类型数 `COUNT(DISTINCT type_qn)` = 934（顶层 934，嵌套已归并） |
| `package_total` | `role='namespace'` 的节点数（原实现硬编码 `level=1`，M3） |
| `source_file_total` | 源码文件数（933） |
| `type_edge_total` | `COUNT(*) FROM type_edge` |
| `method_edge_total` | `COUNT(*) FROM method_edge` |
| `cycle_group_total` | `COUNT(*) FROM cycle_group` |
| `default_pkg_element_total` | `namespace_qn` 为空或 `= '(default)'` 的元素数（295，M4）。analyzer 修复后 Java 默认包元素直出 `(default)`，空值分支仅兼容旧产物与 C++ |

---

## 5. 案例数据（AviatorUtil）

### 5.1 `elements`（9 行）

| qn | kind | name | namespace_qn | enclosing_qn | type_qn | start | end | signature | extra（节选） | is_source | is_sugar | is_impl_detail |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `...utils.AviatorUtil` | CLASS | AviatorUtil | `...utils` | `...utils` | `...AviatorUtil` | 23 | 40 | `public class AviatorUtil` | `{"Modifiers":["public"],"Annotation":["@Slf4j"]}` | T | F | F |
| `...AviatorUtil.getVariables(String)` | METHOD | getVariables | `...utils` | `...AviatorUtil` | `...AviatorUtil` | 31 | 39 | `public static List<String> getVariables(@NonNull String expression)` | `{"Mores":{"java.method.is_constructor":false,"java.method.return_type":"List<String>","java.method.parameters":[...]}}` | T | F | F |
| `...getVariables(String).expression` | VARIABLE | expression | `...utils` | `...getVariables(String)` | `...AviatorUtil` | 31 | 31 | `String expression` | `{"Mores":{"java.variable.is_param":true,"java.variable.raw_type":"String"}}` | T | F | F |
| `...getVariables(String).block$1` | SCOPE_BLOCK | block | `...utils` | `...getVariables(String)` | `...AviatorUtil` | 32 | 35 | `{...}` | — | T | F | **T** |
| `...block$1.compile` | VARIABLE | compile | `...utils` | `...getVariables(String).block$1` | `...AviatorUtil` | 33 | 33 | `Expression compile` | `{"Mores":{"java.variable.raw_type":"Expression"}}` | T | F | F |
| `...getVariables(String).block$2` | SCOPE_BLOCK | block | `...utils` | `...getVariables(String)` | `...AviatorUtil` | 35 | 38 | `{...}` | — | T | F | **T** |
| `...block$2.e` | VARIABLE | e | `...utils` | `...getVariables(String).block$2` | `...AviatorUtil` | 35 | 35 | `ExpressionRuntimeException e` | `{"Mores":{"java.variable.raw_type":"ExpressionRuntimeException"}}` | T | F | F |
| `...AviatorUtil.AviatorUtil()` | METHOD | AviatorUtil | `...utils` | `...AviatorUtil` | `...AviatorUtil` | 23 | 40 | `public AviatorUtil()` | `{"Mores":{"java.method.is_constructor":true,"java.method.is_implicit":true}}` | **F** | **T** | F |
| `...AviatorUtil.log` | FIELD | log | `...utils` | `...AviatorUtil` | `...AviatorUtil` | 23 | 40 | `private static final Logger log` | `{"Mores":{"java.field.is_static":true,"java.lombok.annotation_type":"Slf4j"}}` | **F** | **T** | F |

> 最后两行：`@Slf4j` 生成的 `log` 字段和隐式默认构造器，`is_source=false / is_sugar=true`，但**仍有行号**（落在类声明行 23–40），L3 源码树据此定位。
>
> `extra` 列存的是完整 JSON（`Modifiers` / `Annotation` / `Mores`），表中只节选前端会用到的键。

### 5.2 `relations`（触及该类型的 9 条非 CONTAIN）

| type | source_qn | source_kind | target_qn | target_kind | path | line | raw_text |
|---|---|---|---|---|---|---|---|
| ASSIGN | `...getVariables(String)` | METHOD | `...block$1.compile` | VARIABLE | AviatorUtil.java | 33 | `compile = AviatorEvaluator.compile(expression)` |
| USE | `...getVariables(String)` | METHOD | `...getVariables(String).expression` | VARIABLE | AviatorUtil.java | 33 | `Expression compile = AviatorEvaluator.compile(expression);` |
| USE | `...getVariables(String)` | METHOD | `...block$1.compile` | VARIABLE | AviatorUtil.java | 34 | `return compile.getVariableNames();` |
| USE | `...getVariables(String)` | METHOD | `...AviatorUtil.log` | FIELD | AviatorUtil.java | 36 | `log.error(MessageUtils.getMessage(e));` |
| USE | `...getVariables(String)` | METHOD | `...block$2.e` | VARIABLE | AviatorUtil.java | 36 | `log.error(MessageUtils.getMessage(e));` |
| CALL | `...getVariables(String)` | METHOD | `...MessageUtils.getMessage(Throwable)` | METHOD | AviatorUtil.java | 36 | `MessageUtils.getMessage(e)` |
| CALL | `...IndexDataBuilder.getStringInfo(BaseData,SubRule)` | METHOD | `...getVariables(String)` | METHOD | IndexDataBuilder.java | 264 | `AviatorUtil.getVariables(rule)` |
| IMPORT | `.../IndexDataBuilder.java` | FILE | `...AviatorUtil` | CLASS | IndexDataBuilder.java | 16 | — |
| RETURN | `...AviatorUtil.AviatorUtil()` | METHOD | `...AviatorUtil` | CLASS | — | — | — |

另有 CONTAIN 9 条：`PACKAGE→CLASS`、`CLASS→METHOD×2`、`CLASS→FIELD`、`METHOD→VARIABLE×2`、`METHOD→SCOPE_BLOCK×2`、`SCOPE_BLOCK→VARIABLE`。

> `source_kind` / `target_kind` 在 Java 里看起来冗余（QN 已唯一），但 C++ 的 `IMPL_LINK` 两端 QN 相同、只有 Kind 不同，所以这两列是**必填**的。

### 5.3 `dimensions`

| id | language | qn_separator | imported_at |
|---|---|---|---|
| 1 | java | `.` | 2026-11-11 12:55:00 |

### 5.4 `dimension_levels`

| dimension_id | level | role | kinds |
|---|---|---|---|
| 1 | 1 | namespace | `PACKAGE` |
| 1 | 2 | type | `CLASS,INTERFACE,ENUM,ANNOTATION` |
| 1 | 3 | member | `METHOD,FIELD,ENUM_CONSTANT,VARIABLE,SCOPE_BLOCK,LAMBDA,METHOD_REF,ANONYMOUS_CLASS` |

### 5.5 `hierarchy_node`（本案例相关 6 行）

| dimension_id | qn | level | role | parent_qn | depth | kind |
|---|---|---|---|---|---|---|
| 1 | `com` | 1 | namespace | `` | 0 | PACKAGE |
| 1 | `com.huawei` | 1 | namespace | `com` | 1 | PACKAGE |
| 1 | `com.huawei.tenet` | 1 | namespace | `com.huawei` | 2 | PACKAGE |
| 1 | `com.huawei.tenet.common` | 1 | namespace | `com.huawei.tenet` | 3 | PACKAGE |
| 1 | `com.huawei.tenet.common.utils` | 1 | namespace | `com.huawei.tenet.common` | 4 | PACKAGE |
| 1 | `...utils.AviatorUtil` | 2 | type | `...utils` | 5 | CLASS |

### 5.6 `element_node`（9 行）

| element_qn | element_kind | dimension_id | node_qn | level | 挂载依据 |
|---|---|---|---|---|---|
| `...AviatorUtil` | CLASS | 1 | `...AviatorUtil` | 2 | `type_qn`（自身） |
| `...getVariables(String)` | METHOD | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...getVariables(String).expression` | VARIABLE | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...getVariables(String).block$1` | SCOPE_BLOCK | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...block$1.compile` | VARIABLE | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...getVariables(String).block$2` | SCOPE_BLOCK | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...block$2.e` | VARIABLE | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...AviatorUtil.AviatorUtil()` | METHOD | 1 | `...AviatorUtil` | 3 | `type_qn` |
| `...AviatorUtil.log` | FIELD | 1 | `...AviatorUtil` | 3 | `type_qn` |

> 关键：`block$1.compile` 的 `enclosing_qn` 是 `block$1`，但 `node_qn` 是 `AviatorUtil`——**成员归属用 `type_qn`，不是 `enclosing_qn`**。

### 5.7 `type_edge`（触及该类型的 2 行）

| dimension_id | source_qn | target_qn | types | edge_count | site_count | cross_pkg |
|---|---|---|---|---|---|---|
| 1 | `...utils.AviatorUtil` | `...utils.MessageUtils` | `[call]` | 1 | 1 | **false** |
| 1 | `...repovizdefectdetail.IndexDataBuilder` | `...utils.AviatorUtil` | `[call]` | 1 | 1 | **true** |

> 类内部的 5 条 USE/ASSIGN 全部因 `source.type_qn == target.type_qn` 被过滤（自环不计）。IMPORT 被排除。

### 5.8 `call_site`（7 行，全部有行号）

| dimension_id | source_qn | source_kind | target_qn | target_kind | type | path | line | call_count |
|---|---|---|---|---|---|---|---|---|
| 1 | `...getVariables(String)` | METHOD | `...block$1.compile` | VARIABLE | assign | AviatorUtil.java | 33 | 1 |
| 1 | `...getVariables(String)` | METHOD | `...expression` | VARIABLE | use | AviatorUtil.java | 33 | 1 |
| 1 | `...getVariables(String)` | METHOD | `...block$1.compile` | VARIABLE | use | AviatorUtil.java | 34 | 1 |
| 1 | `...getVariables(String)` | METHOD | `...log` | FIELD | use | AviatorUtil.java | 36 | 1 |
| 1 | `...getVariables(String)` | METHOD | `...block$2.e` | VARIABLE | use | AviatorUtil.java | 36 | 1 |
| 1 | `...getVariables(String)` | METHOD | `...MessageUtils.getMessage(Throwable)` | METHOD | call | AviatorUtil.java | 36 | 1 |
| 1 | `...IndexDataBuilder.getStringInfo(...)` | METHOD | `...getVariables(String)` | METHOD | call | IndexDataBuilder.java | 264 | 1 |

### 5.9 `method_edge`（VIEW，3 行）

| dimension_id | source_qn | target_qn | types | edge_count |
|---|---|---|---|---|
| 1 | `...getVariables(String)` | `...MessageUtils.getMessage(Throwable)` | `[call]` | 1 |
| 1 | `...IndexDataBuilder.getStringInfo(BaseData,SubRule)` | `...getVariables(String)` | `[call]` | 1 |
| 1 | `...getVariables(String)` | `...AviatorUtil.log` | `[use]` | 1 |

> 第 3 行说明：**FIELD 也在 member 白名单里**，所以「方法读字段」会成为方法级边。

### 5.10 `type_fan`（3 行示例）

| dimension_id | qn | kind | fan_in | fan_out | fan_in_types | fan_out_types | fan_in_sites | fan_out_sites |
|---|---|---|---|---|---|---|---|---|
| 1 | `...utils.AviatorUtil` | CLASS | 1 | 1 | 1 | 1 | 1 | 1 |
| 1 | `...utils.MessageUtils` | CLASS | 132 | 0 | 63 | 0 | 132 | 0 |
| 1 | `...repovizdefectdetail.IndexDataBuilder` | CLASS | 3 | **168** | 1 | 26 | 3 | **153** |

> `IndexDataBuilder` 是「加权 168 / 去重 153」的典型：同一行调多次会被加权放大。

### 5.11 `violation_config`（默认 1 行）

| dimension_id | fan_warn | fan_severe | cycle_relation_types | merge_nested | dedupe_same_site | updated_at |
|---|---|---|---|---|---|---|
| 1 | 50 | 150 | `[CALL,CREATE,EXTEND,IMPLEMENT,TYPE_ARG,CAPTURE,USE,ASSIGN,CAST,PARAMETER,RETURN,THROW,ANNOTATION]` | true | true | 2026-11-11 12:55:00 |

### 5.12 `violation_hit`（Top 5 示例，`dedupe_same_site=true`）

| dimension_id | qn | metric | severity | value | type_count |
|---|---|---|---|---|---|
| 1 | `...utils.ServiceException` | fanin | severe | 582 | 139 |
| 1 | `...entity.Task` | fanin | severe | 577 | 67 |
| 1 | `...constants.Constant` | fanin | severe | 408 | 147 |
| 1 | `...pojo.TenetResult` | fanin | severe | 386 | 35 |
| 1 | `...entity.dataflow.CodecheckJob` | fanin | severe | 351 | 31 |

`dedupe_same_site=false` 时 `value` 分别变为 1,697 / 680 / 471 / 392 / 385。

### 5.13 `cycle_group` + `cycle_member` + `cycle_edge`（全库唯一跨包环）

`cycle_group`

| dimension_id | group_id | member_count | pkg_count |
|---|---|---|---|
| 1 | 1 | 2 | 2 |

`cycle_member`

| dimension_id | group_id | type_qn | namespace_qn |
|---|---|---|---|
| 1 | 1 | `...utils.ProjectUtils` | `...common.utils` |
| 1 | 1 | `...entity.Project` | `...common.entity` |

`cycle_edge`

| dimension_id | group_id | source_qn | target_qn | types | site_count |
|---|---|---|---|---|---|
| 1 | 1 | `...entity.Project` | `...utils.ProjectUtils` | `[call]` | 1 |
| 1 | 1 | `...utils.ProjectUtils` | `...entity.Project` | `[call,parameter]` | 3 |

> `ProjectUtils → Project` 的 3 个引用点：`ProjectUtils.java:108` 与 `:111`（`findGitFolder` → `setGitFilePath`，CALL），加 1 条无行号的 `PARAMETER`（`findGitFolder` 的形参类型，按 `NOLOC:source_qn` 计 1 点）；`Project → ProjectUtils` 1 个：`Project.java:289`。

> 另有 2 个包内环（`EnumValidator ↔ EnumValid`、`UuidsValidator ↔ UuidsValid`）因 `COUNT(DISTINCT namespace_qn) < 2` 被丢弃。

### 5.14 `pkg_rollup`（祖先链 4 行）

| dimension_id | qn | subtree_type_count | subtree_cycle_groups | subtree_high_fanin | subtree_high_fanout | direct_type_count |
|---|---|---|---|---|---|---|
| 1 | `com.huawei.tenet.common.utils` | 44 | 1 | 4 | 2 | **31** |
| 1 | `com.huawei.tenet.common` | 382 | 1 | 12 | 9 | 0 |
| 1 | `com.huawei.tenet` | 925 | 1 | 79 | 99 | 0 |
| 1 | `com` | 925 | 1 | 79 | 99 | 0 |

> `subtree_type_count` 按**顶层类型**（`type_qn = qn`）计数，嵌套类已归并到宿主类型，因此根 `com` 是 925 而非 1,059。`common` / `tenet` / `com` 的 `direct_type_count=0`（只有子包无直接类型）。默认包的 9 个类型挂在独立的 `(default)` 节点下（`subtree_type_count = direct_type_count = 9`），不与 `com` 合并，因此根仍为 925。

### 5.15 `metrics`

| dimension_id | kind | count |
|---|---|---|
| 1 | element_total | 35,706 |
| 1 | relation_total | 100,321 |
| 1 | package_total | 253 |
| 1 | type_total | 934 |
| 1 | type_element_total | 1,059 |
| 1 | source_file_total | 933 |
| 1 | type_edge_total | 3,549 |
| 1 | method_edge_total | 17,314 |
| 1 | cycle_group_total | 1 |
| 1 | default_pkg_element_total | 295 |

---

## 6. 补充案例：嵌套类与默认包

### 6.1 嵌套类归并（`LoginVo.Builder`）

| qn | kind | namespace_qn | enclosing_qn | type_qn | is_sugar |
|---|---|---|---|---|---|
| `...entity.LoginVo.Builder` | CLASS | `...entity` | `...entity.LoginVo` | `...entity.LoginVo` | true |
| `...LoginVo.Builder.userAccount(String)` | METHOD | `...entity` | `...LoginVo.Builder` | `...entity.LoginVo` | true |

> `Builder` 不作为独立类型节点参与 `type_edge`/环；它和 `LoginVo` 的相互调用因 `type_qn` 相同被过滤，消除了旧口径里的假环。

### 6.2 默认包（无 `package` 声明）

| qn | kind | path | namespace_qn | enclosing_qn | type_qn |
|---|---|---|---|---|---|
| `OperationDataTest` | CLASS | `cleanarch-backend-admin/src/test/java/OperationDataTest.java` | `(default)` | 空 | `OperationDataTest` |
| `OperationDataTest.sdf` | FIELD | 同上 | `(default)` | `OperationDataTest` | `OperationDataTest` |

> 9 个测试文件共 295 个元素。analyzer 现已直出 `namespace_qn = "(default)"` 并注册 `(default)` PACKAGE 元素，这些元素自然进入 `hierarchy_node` 与 `pkg_rollup`；后端 `COALESCE` 兜底仅用于旧产物与 C++。

### 6.3 C++ 声明/定义共享 QN（`(qn, kind)` 主键的直接证据）

`x/cpp/testdata/collector/cpp/decl_def` 实测 25 个元素，3 个 QN 重复：

| qn | kind | path | line | 说明 |
|---|---|---|---|---|
| `DeclaredClass::declared_method()` | `METHOD_DECL` | decl_def.h | 15 | 类内声明 |
| `DeclaredClass::declared_method()` | `METHOD` | decl_def.cpp | 31 | 类外定义 |
| `header_func(int)` | `FUNCTION_DECL` | decl_def.h | 19 | 头文件原型 |
| `header_func(int)` | `FUNCTION` | decl_def.cpp | 26 | 定义 |
| `local_proto(int)` | `FUNCTION_DECL` | decl_def.cpp | 5 | 同文件先声明 |
| `local_proto(int)` | `FUNCTION` | decl_def.cpp | 6 | 后定义 |

对应的 `IMPL_LINK` 关系（两端 QN 完全相同，只能靠 Kind 区分）：

| type | source_qn | source_kind | target_qn | target_kind |
|---|---|---|---|---|
| IMPL_LINK | `DeclaredClass::declared_method()` | `METHOD` | `DeclaredClass::declared_method()` | `METHOD_DECL` |
| IMPL_LINK | `header_func(int)` | `FUNCTION` | `header_func(int)` | `FUNCTION_DECL` |
| IMPL_LINK | `local_proto(int)` | `FUNCTION` | `local_proto(int)` | `FUNCTION_DECL` |

> 若 `relations` 只存 `source_qn/target_qn`，这三条关系永远无法 JOIN 到正确元素。`IMPL_LINK` 归入 `structural_relation_types`，不参与依赖边统计。

### 6.4 C++ 无 `type_qn` 的元素（挂载回退）

`decl_def` 样本中 `type_qn` 为空的 kind：

| kind | 数量 | `namespace_qn` | 回退挂载到 |
|---|---|---|---|
| `FUNCTION` / `FUNCTION_DECL` | 3 | 空（全局） | 文件节点 |
| `MACRO` | 2 | 空 | 文件节点 |
| `TYPE`（typedef） | 1 | 空 | 文件节点 |
| `NAMESPACE` | 1 | 空 | level 1 节点自身 |

若代码在 `namespace app { int f(); }` 内，则 `namespace_qn = app`，`FUNCTION` 挂到 `app` 命名空间节点（level 1），而不是文件节点。

---

## 7. 容易踩的细节

| 细节 | 说明 |
|---|---|
| **`qn` 不能单独做主键** | C++ 声明/定义共享 QN（e2e 样本 1,417 元素中 76 个重复，72 个 decl+def 对）。必须用 `(qn, kind)` |
| **`relations` 必须带端点 kind** | C++ `IMPL_LINK` 两端 QN 相同；实测 Java/C++ 关系端点 100% 带 `Kind`，无需 analyzer 改动 |
| **跨文件 static 同名会被丢弃** | 两文件各写 `static int helper(int)` → 第二个元素消失，调用被错误解析到第一个。本轮**接受**；升级路径 = analyzer 修复后改 `(qn,kind,path)` |
| **`element_node` 挂载回退链** | `type_qn` → `namespace_qn` → 文件节点。C++ 的 `FUNCTION`/`MACRO`/`TYPE` 依赖这条链；Java 默认包元素已有 `namespace_qn = "(default)"`，走正常路径 |
| **`type_edge` / `type_fan` 主键** | 已补复合主键；重跑 preprocess 前仍建议 `DELETE WHERE dimension_id=?` |
| **`method_edge` 是 VIEW 不是表** | 不要对它 `INSERT` / `DELETE`；派生口径变化只改 VIEW 定义。`db.Init` 在空库上直接建 VIEW；旧库由 `db.Ensure` 按 `schema_meta.version` 删库重建 |
| **`cross_pkg` 必须在默认包兜底之后算** | 否则旧产物的空 `namespace_qn` 会产生约 40 个假跨包对（P2）；Java 新产物已直出 `(default)` |
| **`types` 数组读回是字符串** | `duckdbthin` 返回 `"[call, use]"`，repository 要 `strings.Trim("[]")` + `Split(", ")` |
| **`element_node` 的类自身行** | 漏了会导致「点开包看不到类」——类必须同时有 `hierarchy_node`（level=2）和 `element_node`（level=2） |
| **`elements.path` 是安全边界** | 只能来自本表；请求传 `qn`，服务端查 `path` 后拼 `manifest.root_path` 并校验 `filepath.Rel` |
| **默认包 9 个类型** | analyzer 直出 `(default)` 后自然进入包树与 DSM；若处理旧产物，不做 `(default)` 兜底会从包树和 DSM 中静默消失 |
| **`violation_hit.value` 的语义** | 它存的是「命中时用的那个值」——`dedupe_same_site=true` 时是 `fan_in_sites`，不是加权值。前端要展示加权值请回查 `type_fan.fan_in` |
| **端点完整性由 analyzer 保证** | `633c58f` 后 18,490 个 `xxx()` 端点全部可解析、外部端点 0 条；后端**不做补全**，JOIN 落空即丢弃 + 告警 |
| **后端不修补解析缺陷** | 跨文件 static 丢弃、C++ 重开 namespace 悬空 `type_qn` 等，一律记录并抛 analyzer（见 [development.md](development.md) §2.3）；默认包归属已由 analyzer 修复 |
| **`IMPL_LINK` 混入依赖边** | 会造出 `FUNCTION → FUNCTION_DECL` 的重复边；必须归入 `structural_relation_types` |
| **717/3,549 类型对无调用点明细** | 纯类型引用（TYPE_ARG 366 / PARAMETER 232 / RETURN 118 / IMPLEMENT 44 / EXTEND 41 / ANNOTATION 31 / THROW 2），边详情要显示「仅类型引用」而非空列表（P1） |
| **`cycle_edge` 与 `call_site` 有 JOIN 断点** | 前者是类型 QN，后者是成员 QN；必须经 `elements.type_qn` 双向中转，不能直接 JOIN（P10） |
| **`type_fan` 要含孤立类型** | 934 类中 22 个无跨类边，也要落全 0 行，否则类型列表缺项（P6） |
