# 架构设计

> 本文回答三个问题：系统由什么组成、数据怎么流动、对外提供哪些接口。开发规范与约束见 [development.md](development.md)。

## 1. 定位与技术选型

**定位**：代码依赖可视化工具。单一命令启动一个 HTTP 服务，同时托管 REST API 与前端页面；用户在页面选择源码目录，后台完成依赖分析，结果落 DuckDB，浏览器按需取当前层切片渲染。

| 层 | 选型 | 说明 |
|---|---|---|
| 入口 | Go 1.25 + `flag` | `cmd/displayer` 是唯一命令入口 |
| HTTP | `go-chi/chi v5` | 轻量路由，无框架侵入 |
| 存储 | DuckDB（cgo 直调 C API） | 列存 + 嵌入式，适合一次写入、多次按层查询 |
| 分析引擎 | `arch-lens-dep-analyer`（replace 到本地目录） | 产出 JSONL，本项目只消费 |
| 配置 | `gopkg.in/yaml.v3` | 维度配置 |
| 前端 | Vue 3 + Vite + TypeScript + Pinia | 无 vue-router，hash 路由 |
| 图渲染 | AntV G6 v5 | 气泡 combo / 蝴蝶固定布局 |
| HTTP 客户端 | Axios | 薄封装 |

**为什么自己写 DuckDB 驱动**：不依赖 go-duckdb，`internal/duckdbthin` 直接调官方 C API，只需随程序提供 `duckdb.dll` / `libduckdb.so`。代价是单连接与 `VARCHAR[]` 绑定限制（见 [development.md](development.md) 约束）。

## 2. 进程与分层

```
┌────────────────────────── 进程边界 ──────────────────────────┐
│  前端（浏览器，Vue3 + G6）                                     │
│    │  REST /api/*                                             │
│    ▼                                                          │
│  api（router → handler → service → dto）                      │
│    │                                                          │
│    ├─► registry（数据源注册表 + 激活切换 + 惰性连接）            │
│    └─► db/repository（全部 SQL，func() *sql.DB 动态取连接）    │
│           │                                                    │
│           ▼                                                    │
│  db（连接生命周期 + Schema 约束）                              │
│    ▲                                                          │
│  pipeline（5 步处理链）──► { preprocess.Preprocessor, ingest.Ingester }  │
│    │                                                          │
│  duckdbthin（薄 cgo 驱动，DuckDB C API 直调）                  │
└──────────────────────────────────────────────────────────────┘
```

包依赖单向、无环（按实际 import 统计）：

```
api → { api/service, api/dto, registry }
api/service → { api/dto, db/repository, pipeline/preprocess, registry }
registry → { db, pipeline }
pipeline → { db, db/repository, pipeline/ingest, pipeline/preprocess }
pipeline/{ingest,preprocess} → db/repository
db → duckdbthin
```

| 包 | 职责 | 关键类型 |
|---|---|---|
| `cmd/displayer` | 唯一命令入口：解析参数、装配 registry/api、托管前端 | `config` |
| `api` | REST 四层：router（路由）/ handler（薄壳）/ service（业务逻辑）/ dto（出入参 schema） | `Server` |
| `registry` | 数据源集合管理：列表、激活切换、惰性打开当前激活源的 DuckDB 连接 | `Registry` |
| `pipeline` | 单数据源 5 步处理链：状态机 + 耗时统计 + meta.json 持久化 + 断点续跑 | `Pipeline` / `Step` |
| `pipeline/ingest` | 写对象：JSONL 导入原始表 | `Ingester` |
| `pipeline/preprocess` | 写对象：维度配置模型 + 层级树 + 全部预计算 | `Preprocessor` / `Dimension` |
| `db` | 连接生命周期 + Schema 约束 | `Open` / `Recreate` / `Ensure` |
| `db/repository` | 全部 SQL（按主表分文件）：读路径随激活源切换，写路径固定连接 | `Repository` |
| `duckdbthin` | 薄 cgo 驱动：DuckDB C API 直调，`database/sql` driver | `Driver` |

### 2.1 读写路径分离

| 维度 | 写路径 | 读路径 |
|---|---|---|
| 对象 | `ingest.Ingester` / `preprocess.Preprocessor` | `repository.Repository` |
| 生命周期 | 一次性（pipeline 步骤内构造使用） | 常驻（随服务存活，随激活源切换） |
| 使用方 | `pipeline`（编排层） | `api.service`（逻辑层） |
| 关心的事 | 把数据算好写回 DuckDB | 从 DuckDB 按需读当前层切片 |

`repository` 有两个入口：`New(func() *sql.DB)` 供读路径（连接随激活源动态取），`Of(*sql.DB)` 供写路径（固定连接）。**所有 SQL 都收敛在 `db/repository`**，pipeline 与 preprocess 不直接操作 `*sql.DB`。

## 3. 数据源与维度

### 3.1 多数据源

每个数据源有独立工作区 `sources/{sid}/`（sid = sha1(源码路径)[:12]），互不干扰：

```
.arch_lens/sources/{sid}/
├── meta.json                 # pipeline 状态（断点续跑 + 前端进度轮询）
├── process.log               # 分析过程日志（每次分析截断重写；见 §5.4）
├── output/                   # 不可变分析产物（语言无关）
│   ├── manifest.json         # 版本/语言/统计/哈希/分隔符/root_path
│   ├── element.jsonl         # 元素
│   └── relation_<type>.jsonl # 关系按类型分文件
└── lens.duckdb               # 派生库（可整库重建）
```

**JSONL 是唯一事实来源，DuckDB 是派生物**：删库可重建，换 schema 只需重跑 ingest + preprocess，不必重新分析源码。

`registry` 维护源列表与当前激活源；API 的所有维度数据请求只面向激活源。连接管理采用**惰性打开 + 缓存**：`ActiveDB()` 首次调用时打开 `lens.duckdb` 并缓存；激活源切换时 `Refresh()` 关闭旧连接，下次请求重新打开。`repository` 持有 `func() *sql.DB` 而非固定连接，无需重建即可跟随切换。

### 3.2 维度配置驱动

层级与关系类型不硬编码，由 `config/dimensions/{language}.yaml` 驱动（`ARCH_LENS_DIM` 可覆盖路径）。`pipeline/preprocess.Dimension` 是纯模型（无 DB 依赖）。

```yaml
language: java
qn_separator: "."
hierarchy:
  - level: 1
    role: namespace
    kinds: [PACKAGE]
  - level: 2
    role: type
    kinds: [CLASS, INTERFACE, ENUM, ANNOTATION]
  - level: 3
    role: member
    kinds: [METHOD, FIELD, ENUM_CONSTANT, VARIABLE, SCOPE_BLOCK, LAMBDA, METHOD_REF, ANONYMOUS_CLASS]
relation_types: [CALL, CREATE, EXTEND, IMPLEMENT, ...]
method_kinds: [METHOD, FIELD, ENUM_CONSTANT, LAMBDA, METHOD_REF, SCOPE_BLOCK]
structural_relation_types: [CONTAIN, IMPORT]
```

- `role` 只有 `namespace` / `type` / `member`，跨语言不变；**层级号只出现在配置里**，SQL 按 `role` 查询。
- `kinds` = 本层接受的元素 kind 白名单（取代旧的 `kind` + `parent_of`）。
- `method_kinds` = 方法级边两端 kind 白名单（可窄于 member 层）。
- `structural_relation_types` 只用于结构/溯源，不参与依赖边统计。

各语言的关系类型全集与边计算口径见 [languages/java.md](languages/java.md)、[languages/cpp.md](languages/cpp.md)。

## 4. 数据模型

DuckDB 表分三层（DDL 见 `internal/db/schema.go`，逐表详解与真实数据见 [schema.md](schema.md)）：

| 层 | 表 | 写入方 |
|---|---|---|
| ① 原始层（语言无关） | `elements`（主键 `(qn, kind)`）、`relations`（带两端 kind） | `ingest` |
| ② 维度层 | `dimensions`、`dimension_levels`、`hierarchy_node`、`element_node` | `preprocess` |
| ③ 预计算层 | `type_edge`、`call_site`、`method_edge`(VIEW)、`type_fan`、`violation_config`、`violation_hit`、`cycle_group`、`cycle_member`、`cycle_edge`、`pkg_rollup`、`metrics`、`annotations` | `preprocess`（`annotations` 由 API 写） |

`elements` 的三个归属字段全部由 analyzer 直接给出，后端原样落列、不做推导：

| 字段 | 语义 | 层级 |
|---|---|---|
| `namespace_qn` | 所属命名空间容器（Java package / C++ namespace / module） | L1 |
| `enclosing_qn` | 直接词法外层（类 / 方法 / lambda / 匿名类 / 块） | L2 |
| `type_qn` | 最外层具名类型，用于嵌套类归并、类型级边、环投影 | 派生语义 |

> 为什么必须由解析器给归属：从 QN 字符串反推必然是猜测（嵌套类会被当成包、C++ `::` 与 Go 路径分隔符会让大小写规则失效）。详见 [development.md](development.md) 的「归属字段」约束。

`schema_meta.version` 与 `db.SchemaVersion` 不符时，`Ensure` 会删库重建（不做旧库迁移）。

## 5. 处理链与预计算

### 5.1 5 步处理链

```
scan → analyze（分析 + 导出 JSONL）→ ingest → preprocess → activate
```

| 步骤 | 职责 | 产出 |
|---|---|---|
| scan | 扫描源码文件 | 源文件清单 |
| analyze | 内嵌 analyzer 引擎做依赖分析，并导出 JSONL | `output/*.jsonl` + 元素/关系统计 |
| ingest | JSONL → DuckDB 原始表（`ingest.Ingester`） | `elements` / `relations` |
| preprocess | 维度/层级/预计算（`preprocess.Preprocessor`） | 维度层 + 预计算层 |
| activate | 数据源标记就绪 | — |

每步状态/耗时持久化到 `meta.json`；中断后 Resume 从失败步骤继续。

### 5.2 preprocess 内部顺序

| 步骤 | 产出 |
|---|---|
| 层级树 | `hierarchy_node` / `element_node`（按 `role` 建树，归属取 `type_qn` → `namespace_qn` → 文件节点） |
| 类型级边 | `type_edge`（`type_qn` 归一化 + 去重引用点数 `site_count`） |
| 调用点 | `call_site`（逐条保留 path/line/type + 两端 kind） |
| 扇入扇出 | `type_fan`（加权值 + 去重类型数 + 去重引用点数，与阈值无关） |
| 违规判定 | `violation_hit`（按 `violation_config` 阈值命中） |
| 环 | `cycle_group` / `cycle_member` / `cycle_edge`（宿主类型归一化 + Tarjan + 仅跨包） |
| 包上卷 | `pkg_rollup`（沿 `parent_qn` 自底向上累加） |
| 指标 | `metrics`（10 项规模指标） |

`method_edge` 是 VIEW，由 `call_site` 聚合，两端 kind 均需在 `method_kinds` 白名单内。

### 5.3 预计算换查询

导入阶段一次性算好并写回上述预计算表；查询阶段只读这些表，避免全量扫描。

**违规口径是服务端唯一事实来源**：`type_fan` 是与阈值无关的原始指标；`violation_config` 存口径（默认 `fan_warn=50` / `fan_severe=150` / 全部环关系类型 / 嵌套归并 / 同点去重）；`violation_hit` / `cycle_*` / `pkg_rollup` 是当前口径的派生结果。前端调阈值走 `PUT /violations/config`，服务端落库后同步重算这三类派生表并返回新结果，`type_edge` / `type_fan` 不动。

**发生点在类型（类）级**：扇入/扇出/环都在 `type_qn` 归一化后判定，包层只做子树向上汇聚。

### 5.4 过程日志与异常告警（process.log）

每个数据源工作区有 `process.log`，记录 `analyze（含导出）→ ingest → preprocess` 的过程与异常，用于**通过消费结果发现 analyzer 产出的元素/关系错误**。由 `internal/pipeline/proclog` 写入，每次分析开始时截断重建。

行格式：`<RFC3339> <LEVEL> [<阶段>:<检查项>] k=v ...`，`LEVEL` 为 `INFO` / `WARN` / `ERROR` / `SUMMARY`。

| 阶段 | 检查项 | 级别 |
|---|---|---|
| analyze | 元素/关系规模、三字段覆盖率、语法糖计数 | SUMMARY |
| analyze | 元素 kind 分布、关系类型直方图 | INFO |
| analyze | 空 QN 元素、空/缺失端点、未知 kind | WARN |
| export | 每个 jsonl 的行数与字节数、总量 | INFO / SUMMARY |
| ingest | JSONL 解析失败（记录后**跳过该行**，不中断导入） | ERROR |
| ingest | 空字段、重复元素 `(qn, kind)` | WARN |
| ingest | 端点悬空（qn+kind 不存在）、端点 kind 不匹配 | WARN（逐条明细） |
| preprocess | 游离元素（无 `element_node`）、游离类型（`fan_in=fan_out=0`） | WARN |
| preprocess | 悬空归属（`namespace_qn`/`enclosing_qn`/`type_qn` 指向不存在元素） | WARN |
| preprocess | 层级孤儿（`parent_qn` 不存在）、未知关系类型 | WARN |
| preprocess | 默认包元素（`namespace_qn` 为空） | WARN |
| preprocess | 派生表行数为 0（hierarchy_node/element_node/type_edge/type_fan/metrics） | ERROR |
| preprocess | 各派生表行数、环组数 | INFO / SUMMARY |

约定：

- **每项明细最多 200 条**，超出只写汇总（`total` 是完整计数，`emitted` 是已写条数）。
- 每个阶段结束必写一行 `SUMMARY`，是最省力的消费入口。
- 只读检查、不修补数据（见 [development.md](development.md) §2.3）。
- 日志写失败降级为 stderr，不影响分析结果。

### 5.5 源码按需读盘

源码内容不入 DuckDB。`GET /source?qn=` 时查 `elements` 拿 path + 元素树 → 从 `manifest.root_path` 拼绝对路径读文件 → 进程内 LRU 缓存。path 必须来自 `elements` 已登记值（防目录穿越）。

## 6. REST API

基址 `/api`。维度数据请求需先激活数据源（无激活源返回 503）。

### 6.1 数据源与目录

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/sources` | 源列表（含步骤进度） |
| POST | `/sources` | 注册源码目录并后台启动 5 步分析（唯一 pipeline 入口） |
| GET | `/sources/{sid}` | 单源状态（前端轮询） |
| POST | `/sources/{sid}/activate` | 切换激活源 |
| POST | `/sources/{sid}/reanalyze` | 清理旧产物（`output/`、`lens.duckdb`、`meta.json`）并重新执行完整分析链 |
| GET | `/fs/list?path=` | 目录浏览（空 path 列盘符） |

### 6.2 维度与全景

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/dimensions` | 维度列表（层级 + 关系类型） |
| GET | `/dimensions/{id}/overview` | 首页全景：指标 + 违规摘要 + 包节点/边/树 |

### 6.3 包树下钻与邻域

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/nodes/{qn}/children` | 下一层子节点 |
| GET | `/{id}/nodes/{qn}/graph?types=` | 一跳邻域边 |
| GET | `/{id}/nodes/{qn}/fanin?depth=N` | 单向扇入 BFS（兼容旧契约） |
| GET | `/{id}/nodes/{qn}/fanout?depth=N` | 单向扇出 BFS（兼容旧契约） |
| GET | `/{id}/top?level=N` | 指定层级顶层节点（兼容旧契约） |

### 6.4 边与类型

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/type-edges?types=` | 全量类型级边 |
| GET | `/{id}/method-edges` | 全量方法级边 |
| GET | `/{id}/calls?source=&target=` | 类型对调用点明细 |
| GET | `/{id}/method-calls?source=&target=` | 方法对调用点明细 |
| GET | `/{id}/types?pkg=` | 包直接类型 + 成员 |
| GET | `/{id}/leaf?pkg=` | 叶子包一次性负载 |
| GET | `/{id}/kind?qn=` | 元素 kind |

### 6.5 影响面与违规

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/fan?qn=&granularity=&depth=&types=&hideImplDetail=` | 双向 BFS 蝴蝶图（`type` / `package` / `method`） |
| GET | `/{id}/fan-stats` | 全量类型扇入扇出原始指标 |
| GET | `/{id}/violations` | 当前口径 + 命中列表 + 环组 |
| GET/PUT | `/{id}/violations/config` | 口径读写（PUT 触发派生表重算） |

### 6.6 源码、统计与标注

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/source?qn=` | 源码内容 + 元素树 |
| GET | `/{id}/nodes/{qn}/source` | 源码定位（path / line） |
| GET | `/{id}/metrics` | 规模指标 map |
| GET | `/{id}/scc` | 环组列表 |
| GET | `/{id}/search?q=&limit=` | 元素模糊搜索 |
| GET | `/{id}/meta` | 数据可信度信息 |
| GET | `/{id}/topby?by=&limit=` | 指标排序 TOP |
| GET/PUT/DELETE | `/{id}/annotations[/{qn}]` | 收藏与备注 |

> `/metrics`、`/scc`、`/search`、`/meta`、`/topby`、`/annotations` 目前无前端调用方，保留为后端能力。

### 6.7 模块划分

API 按**业务域**分文件，每个模块在 `api/`（handler）、`api/service/`、`api/dto/` 三处同名：

| 模块 | 职责 |
|---|---|
| `source.go` | 数据源管理 + 目录浏览（唯一 pipeline 入口） |
| `dimension.go` | 维度列表 + 全景 |
| `graph.go` | 包树下钻与邻域 |
| `edge.go` | 边与调用点 |
| `type.go` | 类型与叶子包 |
| `fan.go` | 蝴蝶图（扇入扇出） |
| `violation.go` | 违规口径与结果（唯一请求期写派生表） |
| `sourcefile.go` | 源码读盘 |
| `analysis.go` | 指标与检索 |
| `annotation.go` | 收藏与备注 |

共享层 `server.go` / `router.go` / `response.go` 只放装配与工具，不承载业务。

## 7. 前端

### 7.1 目录

```
front/src/
├── main.ts                  # createApp(ProtoApp) + Pinia
├── api/                     # client.ts / sources.ts / dimensions.ts / proto.ts
├── stores/                  # source.ts（源 + 激活 + 轮询）/ dimension.ts（维度）
├── types/index.ts           # 数据源/维度/目录浏览契约类型
└── proto/                   # 唯一前端界面
    ├── ProtoApp.vue         # 外壳：hash 路由 + 视图分发
    ├── router.ts            # #/sources、#/overview、#/l2/{qn}、#/l3/{qn}[/{qn2}]
    ├── protoStore.ts        # 面包屑栈 + 数据缓存（按维度 id 失效）
    ├── fanGraph.ts          # 蝴蝶图数据结构 + 布局
    ├── violation.ts         # 违规结果模型 + 后端结果映射
    ├── types/model.ts       # 视图数据模型（与后端 DTO 同构）
    ├── recentSources.ts     # 最近打开的目录（Cookie）
    ├── components/          # ProtoGraphCanvas / QnCombo / SourcePane
    └── views/               # SourceGateView / L1OverviewView / L3BlastView / L4SourceView
```

> 视图命名沿用历史术语：`L1OverviewView` = 全景，`L3BlastView` = 影响面（路由 `l2`），`L4SourceView` = 源码（路由 `l3`）。

### 7.2 视图与交互

| 视图 | 路由 | 内容 |
|---|---|---|
| 选择源码目录（首页） | `#/sources` | 新建数据源（路径输入 / 目录浏览）+ 最近打开（Cookie）+ 已有数据源列表 + 5 步进度 |
| 仓库全景 | `#/overview` | 气泡 combo（包框/类节点）或 DSM 矩阵；缺陷概览；可调违规口径 |
| 影响面 | `#/l2/{qn}[?m=]` | 蝴蝶图：中心置中、左翼上游、右翼下游；包/类/方法三档；节点与边详情 |
| 源码 | `#/l3/{qn}[/{qn2}]` | 元素树 + 源码 + 行高亮；支持双节点预览与调用点定位 |

关键交互：

- 启动固定进入**选择源码目录**页（不自动激活任何源）；最近打开的目录存 Cookie（最多 8 条，同路径去重置顶），已分析过的直接进入，未分析的填入路径待分析。
- 「最近打开」与「已有数据源」都提供**重新分析**按钮：确认后清理该源旧产物并重跑 5 步，完成后自动激活进入。
- 顶部「切换数据源」按钮可从任意分析页回到选择页；品牌按钮在有激活源时回全景、否则回选择页。
- 全景中**单击** = 选中看详情，**双击**包 = 就地展开为主框，**右键** = 下钻 / 折叠 / 查看源码；可同时保留多个主框。
- 违规视觉：红描边 = 跨包环、橙描边 = 高扇入、绿描边 = 高扇出。
- 影响面可折叠实现细节（lambda / 匿名类 / 代码块），边可展开到调用点级。
- 源码橙色徽标 = 编译器隐式生成（`is_sugar`）。

### 7.3 URL 记忆

视图与下钻状态序列化到 hash，刷新/分享可恢复：

```
#/sources
#/overview
#/l2/{qn}[?m={method}]
#/l3/{qn}[/{qn2}][?line=..&sq=..&tq=..]
```

无 hash 时默认 `#/sources`。`ProtoApp` 监听 `hashchange`；`route` 变化防抖写回 hash，并用 `selfSync` 标记跳过自写触发的重放。

## 8. 一次完整数据流

```
用户选择源码目录 → POST /api/sources
  → pipeline 后台执行 5 步 → 前端轮询 GET /api/sources/{sid} 显示进度
  → 就绪后自动 POST /api/sources/{sid}/activate

用户进入全景 → GET /api/dimensions/{id}/overview
  → 包节点 + 包间聚合边 + 指标 + 违规摘要
  → 前端 G6 渲染气泡（size=子树类型数），可切 DSM 矩阵

用户双击包 → GET /api/dimensions/{id}/types?pkg=  或  /leaf?pkg=
  → 直接子包 / 类节点 + 类级边 → 主框就地展开

用户进入影响面 → GET /api/dimensions/{id}/fan?qn=&granularity=type|package|method&depth=N
  → 服务端双向 BFS 返回蝴蝶图节点/边/统计

用户查看源码 → GET /api/dimensions/{id}/source?qn=
  → 按 path 读盘返回内容 + 元素树（进程内缓存）
```
