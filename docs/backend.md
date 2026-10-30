# 后端开发指南

> 面向维护者。本仓库后端为 Go，对外不区分前后端（见 [architecture.md](architecture.md)）。本文描述 Go 包结构、DuckDB 数据模型、REST API 契约与常见扩展操作。

## 1. Go 包结构

```
internal/
├── api/            # REST 四层
│   ├── router.go           # 路由注册
│   ├── server.go           # Server 聚合 + 响应工具
│   ├── handler_*.go        # 薄壳 handler（参数解析 + 调 service + 写响应）
│   ├── service/            # 业务逻辑层（调度 registry + repository，映射 dto）
│   └── dto/                # 出入参 schema（与前端契约对齐）
├── registry/       # 数据源注册表（多源 + 激活 + 惰性连接）
├── pipeline/       # 6 步处理链（状态机 + meta.json + Resume）
├── repository/     # 读模型 DAO（约 21 个查询方法）
├── db/             # 数据层（连接 + Schema + 表操作）
├── index/          # 写对象：层级树 + 预计算（Indexer）
├── ingest/         # 写对象：JSONL 导入（Ingester）
├── dimension/      # 纯模型：维度配置（Dimension/Level）
└── duckdbthin/     # 薄 cgo 驱动（DuckDB C API）
```

依赖方向（无环）：`api → {registry, repository}`；`registry → pipeline → {index, ingest} → db`；`repository → db`；`index → dimension`；`db → duckdbthin`。

## 2. 关键流程

### 2.1 数据源分析（6 步 pipeline）

`POST /api/sources` 触发后台分析，pipeline 串行执行：

| 步骤 | 代码 | 产出 |
|---|---|---|
| scan | `pipeline/step_scan.go` | 源文件清单 |
| analyze | `pipeline/step_analyze.go` | 元素/关系（内嵌 analyzer） |
| export | `pipeline/step_export.go` | `output/*.jsonl` |
| ingest | `pipeline/step_ingest.go` → `ingest.Ingester` | `elements` / `relations` 表 |
| preprocess | `pipeline/step_preprocess.go` → `index.Indexer` | 层级/预计算表 |
| activate | `pipeline/step_activate.go` | 数据源就绪 |

每步状态写入 `meta.json`（`pipeline.MetaFile`），支持断点续跑。

### 2.2 激活源切换

`POST /api/sources/{sid}/activate` → `registry.Activate(sid)` → `Refresh()` 关闭旧连接；下次请求 `ActiveDB()` 惰性重开新源连接。`repository` 持 `func() *sql.DB`，随激活源动态取连接，无需重建。

## 3. DuckDB 数据模型

11 张表，分三组：

### 3.1 原始表（语言无关，ingest 写入）

| 表 | 字段 | 说明 |
|---|---|---|
| `elements` | qn, kind, name, path, is_source, is_sugar, is_external, start_line | 代码元素 |
| `relations` | source_qn, target_qn, type, start_line, raw_text | 依赖关系（原始） |

### 3.2 维度与层级表（维度驱动）

| 表 | 字段 | 说明 |
|---|---|---|
| `dimensions` | id (PK), language, name, imported_at | 维度（每语言一条） |
| `dimension_levels` | dimension_id, level, kind, parent_of | 层级定义（来自 yaml） |
| `hierarchy_node` | dimension_id, qn, level, parent_qn, kind | 层级树节点 |
| `element_node` | element_qn, dimension_id, node_qn, level | 元素归属（元素→节点） |

### 3.3 预计算表（index 写回）

| 表 | 字段 | 说明 |
|---|---|---|
| `level_edges` | dimension_id, level, source_qn, target_qn, edge_count, types | 层级聚合边（count=依赖条数，types 逗号串） |
| `fan` | dimension_id, qn, fan_in, fan_out | 类级扇入/扇出 |
| `scc` | dimension_id, component_id, member_qn | 强连通分量 |
| `metrics` | dimension_id, kind, count | 规模指标 |
| `annotations` | dimension_id, qn (PK), starred, note, created_at, updated_at | 节点标注/收藏（D6） |

## 4. REST API

基址 `/api`。维度数据请求需先激活数据源（无激活源返回 503）。

### 4.1 数据源管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/sources` | 源列表（含步骤进度） |
| POST | `/sources` | 创建源并后台分析 `{path, language}` |
| GET | `/sources/{sid}` | 单源状态（轮询用） |
| POST | `/sources/{sid}/activate` | 激活源 |
| GET | `/fs/list?path=` | 目录浏览（空 path 列盘符） |

### 4.2 维度

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/dimensions` | 维度列表（含层级定义） |

### 4.3 图数据（/dimensions/{id}）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/top?level=N` | 顶层节点 + 聚合边 |
| GET | `/{id}/overview` | 首页全景（level=1 包 + 包间边） |
| GET | `/{id}/nodes/{qn}/children` | 下一层子节点 |
| GET | `/{id}/nodes/{qn}/graph?types=` | 焦点子图（邻接边） |
| GET | `/{id}/nodes/{qn}/source` | 源码定位 FilePath:Line |
| GET | `/{id}/nodes/{qn}/fanin?depth=N` | 扇入 BFS 子图 |
| GET | `/{id}/nodes/{qn}/fanout?depth=N` | 扇出 BFS 子图 |

### 4.4 分析

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/scc` | 强连通分量列表 |
| GET | `/{id}/metrics` | 规模指标 map |
| GET | `/{id}/search?q=&limit=` | 元素模糊搜索 |
| GET | `/{id}/meta` | 数据可信度信息 |
| GET | `/{id}/topby?by=fanout\|fanin\|size&limit=` | 指标排序 TOP |

### 4.5 标注/收藏

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/{id}/annotations` | 标注列表 |
| PUT | `/{id}/annotations/{qn}` | upsert `{starred?, note?}`（nil 保留原值） |
| DELETE | `/{id}/annotations/{qn}` | 删除标注 |

### 4.6 通用响应形状

```json
// 图数据
{ "nodes": [{"id","label","kind","size"}], "edges": [{"source","target","type","types","count"}] }
// 错误
{ "error": "..." }
```

## 5. 常见扩展

### 5.1 加新语言维度

1. 新建 `config/dimensions/{lang}.yaml`（参考 java.yaml 的 hierarchy + relation_types）。
2. 启动时通过 `ARCH_LENS_DIM` 环境变量指向该文件，或注册进默认查找逻辑。
3. 数据源创建时 `language` 传对应语言。

### 5.2 加新 API 端点

1. `dto/` 定义请求/响应结构。
2. `service/` 加业务方法（内部调用 repository 或 registry）。
3. `handler_*.go` 加薄壳 handler。
4. `router.go` 注册路由。

### 5.3 加新查询

在 `repository/` 下加方法，直接写 DuckDB SQL 查询预计算表。保持 repository 为纯读模型，不掺业务判断。
