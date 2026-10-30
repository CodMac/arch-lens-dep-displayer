# 架构设计

> 技术视角：本系统对外是一个命令 `arch-lens-dep-displayer`，前后端只是同一进程内的两个实现部分。本文描述实际代码中的分层、依赖方向与关键机制。

## 1. 总览

单一命令启动器：`cmd/displayer` 启动一个 HTTP 服务，同时托管 REST API 与前端静态资源。前端页面选择源码目录后，由 `internal/pipeline` 在后台执行 6 步分析链，结果写入该数据源独立工作区的 DuckDB；浏览器通过 REST API 按需取当前层切片渲染。

```
┌────────────────────────── 进程边界 ──────────────────────────┐
│  前端（浏览器，Vue3 + G6）                                     │
│    │  REST /api/*                                             │
│    ▼                                                          │
│  api（router → handler → service → dto）                      │
│    │                                                          │
│    ├─► registry（数据源注册表 + 激活切换 + 惰性连接）            │
│    └─► repository（读模型 DAO，func() *sql.DB 动态取连接）      │
│           │                                                    │
│           ▼                                                    │
│  db（连接 + 表定义 + 表操作，主表驱动）                          │
│    ▲                                                          │
│  pipeline（6 步处理链）──► { index.Indexer, ingest.Ingester }  │
│    │                                                          │
│    └─► dimension（纯模型：维度配置）                            │
│                                                              │
│  duckdbthin（薄 cgo 驱动，DuckDB C API 直调）                  │
└──────────────────────────────────────────────────────────────┘
```

## 2. 分层与依赖方向

Go 包依赖为单向前进、无环（从 `grep` 实际 import 统计）：

```
api → { registry, repository }
registry → pipeline → { index, ingest }
                       ↘ db
repository → db
index → dimension
ingest → db

db → duckdbthin（驱动，被所有查询/写入间接依赖）
```

### 各包职责

| 包 | 职责 | 关键类型 |
|---|---|---|
| `api` | REST 四层：router（路由）/ handler（薄壳）/ service（业务逻辑）/ dto（出入参 schema） | `Server` |
| `registry` | 数据源集合管理：列表、激活切换、惰性打开当前激活源的 DuckDB 连接 | `Registry` |
| `pipeline` | 单数据源 6 步处理链：状态机 + 耗时统计 + meta.json 持久化 + 断点续跑 | `Pipeline` / `Step` |
| `repository` | 读模型 DAO：面向激活源的全部业务查询（约 21 个方法） | `Repository` |
| `db` | 数据层：连接管理 + Schema 建表 + 主表 CRUD（无业务逻辑） | 函数集 |
| `index` | 写对象：按维度配置构建层级树 + 预计算（SCC/fan/聚合边） | `Indexer` |
| `ingest` | 写对象：JSONL 导入原始表 | `Ingester` |
| `dimension` | 纯模型：维度配置加载与层级定义 | `Dimension` / `Level` |
| `duckdbthin` | 薄 cgo 驱动：DuckDB C API 直调，`database/sql` driver | `Driver` |

## 3. 读写路径分离

系统显式区分「写路径」与「读路径」，生命周期与依赖方向完全不同：

| 维度 | 写路径 | 读路径 |
|---|---|---|
| 对象 | `ingest.Ingester` / `index.Indexer` | `repository.Repository` |
| 生命周期 | 一次性（pipeline 步骤内构造使用） | 常驻（随服务存活，随激活源切换） |
| 使用方 | `pipeline`（编排层） | `api.service`（逻辑层） |
| 关心的事 | 把数据算好写回 DuckDB | 从 DuckDB 按需读当前层切片 |

```
写：pipeline → ingest → db
        └──► index → db（维度/层级/预计算）
读：api → service → repository → db
```

## 4. 数据源与维度

### 4.1 多数据源

每个数据源有独立工作区 `sources/{sid}/`（sid = sha1(源码路径)[:12]），互不干扰。`registry` 维护源列表与当前激活源；API 的所有维度数据请求只面向激活源。

连接管理采用**惰性打开 + 缓存**：`registry.ActiveDB()` 首次调用时打开 `lens.duckdb` 并缓存；激活源切换时 `Refresh()` 关闭旧连接，下次请求重新打开。`repository` 持有 `func() *sql.DB` 而非固定连接，从而无需重建即可跟随激活源切换。

### 4.2 维度配置驱动

层级与关系类型不硬编码在代码中，由 `config/dimensions/*.yaml` 驱动（`ARCH_LENS_DIM` 环境变量可覆盖路径）。`dimension` 纯模型包负责加载；`index.Indexer` 依据配置构建层级树。

Java 示例（`config/dimensions/java.yaml`）：

```yaml
language: java
hierarchy:
  - level: 1
    kind: PACKAGE
    parent_of: [PACKAGE]
  - level: 2
    kind: CLASS
    parent_of: [CLASS, INTERFACE, ENUM]
  - level: 3
    kind: MEMBER
    parent_of: [METHOD, FIELD]
relation_types:
  - CALL
  - CREATE
  - EXTEND
  - IMPLEMENT
  ...
```

## 5. 关键机制

### 5.1 6 步处理链（pipeline）

```
scan → analyze → export → ingest → preprocess → activate
```

| 步骤 | 职责 |
|---|---|
| scan | 扫描源码文件（找到 N 个源文件） |
| analyze | 内嵌 analyzer 引擎做依赖分析（元素/关系统计） |
| export | 导出 JSONL 产物（element.jsonl + relation_*.jsonl） |
| ingest | JSONL → DuckDB 原始表（`ingest.Ingester`） |
| preprocess | 维度/层级/预计算（`index.Indexer`：层级树 + SCC + fan + 聚合边 + metrics） |
| activate | 数据源标记就绪 |

每步状态/耗时持久化到 `meta.json`；中断后可断点续跑（Resume 从失败步骤继续）。

### 5.2 预计算换查询

导入阶段一次性算好并写回：层级树（`hierarchy_node`/`element_node`）、层级聚合边（`level_edges`）、扇入扇出（`fan`）、强连通分量（`scc`）、指标（`metrics`）。查询阶段只读这些预计算表，避免全量扫描。

### 5.3 前端状态与 URL 记忆

前端无 vue-router，视图模式 + 下钻栈由 Pinia store 承载，并序列化到 `location.hash`：

```
#/d/{sid}/{mode}/{level}:{qn}/{level}:{qn}/...
#   mode: overview | level
```

刷新/分享 URL 可从 hash 恢复完整下钻路径；写回与应用双重防环（见 [frontend.md](frontend.md)）。

## 6. 数据流示例（一次下钻）

```
用户点「仓库全景」→ GET /api/dimensions/{id}/overview
  → repository.OverviewNodes + LevelEdges（level=1 顶层包 + 包间聚合边）
  → 前端 G6 渲染包节点（size=包内类数）

用户点某个包 → GET /api/dimensions/{id}/nodes/{qn}/children
  → repository.ChildNodes（下一层类节点）
  → hash 追加 {level}:{qn} → 下钻栈 +1

用户右键类 → 扇入/扇出 → GET /api/dimensions/{id}/nodes/{qn}/fanin|fanout?depth=N
  → repository.FanEdges BFS 收集可达子图
```
