# arch-lens-dep-displayer

依赖可视化系统：单一命令 `arch-lens-dep-displayer` 启动服务，在浏览器中选择源码目录后自动完成「扫描 → 依赖分析 → 导入 DuckDB → 结果预处理 → 展示」，支持多数据源（可切换多个源码目录），并可通过浏览器逐层下钻查看包/类/成员之间的依赖关系。

## 一条命令启动

```bash
arch-lens-dep-displayer
```

启动后自动打开 http://localhost:8080。

1. **新建数据源**：输入源码目录绝对路径（或点「浏览…」逐级选择），点「开始分析」。
2. 后台按固定步骤链执行：**扫描 → 依赖分析 → 导出 JSONL → 导入 DuckDB → 结果预处理 → 数据源就绪**，前端实时显示每步耗时/状态；中断后可断点续跑。
3. 分析完成自动激活并进入**仓库全景**；已分析过的数据源下次启动直接进入（不重复分析）。

## 常用命令

```bash
# 构建可执行文件（先构建前端）
cd front && npm install && npm run build
cd .. && go build -o .bin/arch-lens-dep-displayer.exe ./cmd/displayer

# 启动（默认 :8080，自动开浏览器）
./.bin/arch-lens-dep-displayer

# 指定数据源根目录 / 端口 / 不开浏览器
./.bin/arch-lens-dep-displayer -out ./.arch_lens/sources -listen :9090 -browser=false
```

## 多数据源工作区

```
.arch_lens/sources/{sid}/       # sid = sha1(源码路径)[:12] 稳定哈希
├── meta.json    # 步骤状态/耗时（断点续跑 + 前端进度轮询）
├── output/      # dep-analyer JSONL 产物（element.jsonl + relation_*.jsonl）
└── lens.duckdb  # DuckDB 持久化（原始表 + 维度层级 + 预计算结果）
```

## 组件结构

| 组件 | 位置 | 职责 |
|---|---|---|
| **displayer** | `cmd/displayer` | 唯一命令入口：启动服务 + 托管前端（纯入口，无分析逻辑） |
| **pipeline** | `internal/pipeline` | 6 步处理链（scan→analyze→export→ingest→preprocess→activate），含耗时统计 + meta.json 持久化 + 断点续跑 |
| **registry** | `internal/registry` | 数据源注册表：源列表、激活切换、惰性打开 DuckDB 连接 |
| **api** | `internal/api` | REST API（router/handler/service/dto 四层）：/sources 多源管理 + /fs 目录浏览 + /dimensions 数据接口 |
| **front** | `front/` | 浏览器界面（Vue3 + Vite + TS + AntV G6）：数据源首页 + 仓库全景 + 层级图 |
| internal/ | `internal/{db,repository,ingest,index,dimension,duckdbthin}` | 数据层 / 读模型 / 写对象 / 纯模型 / 薄 cgo 驱动 |

```
源码目录 ──► pipeline（6 步）──► sources/{sid}/lens.duckdb
                                      │
                                      ▼
registry（多源 + 激活）──► api（REST）──► front (浏览器)
```

## 平台依赖：DuckDB 动态库

不依赖 go-duckdb，直接调用 DuckDB 官方 C API（薄 cgo 驱动 [`internal/duckdbthin`](internal/duckdbthin/)）。需要随程序提供对应平台的 DuckDB 动态库：

| 平台 | 目录 | 库 |
|---|---|---|
| Windows amd64 | `deps/windows_amd64/` | `duckdb.dll`（+ import lib `libduckdb.dll.a`） |
| Windows arm64 | `deps/windows_arm64/` | `duckdb.dll` |
| Linux amd64 | `deps/linux_amd64/` | `libduckdb.so` |
| Linux arm64 | `deps/linux_arm64/` | `libduckdb.so` |

运行时：Windows 需把 `duckdb.dll` 放在 exe 同目录（`.bin/` 已含）；Linux 需 `LD_LIBRARY_PATH` 指向 deps 对应目录。

cgo 编译路径由 `internal/duckdbthin/cgo_*.go` 的 build-tag 文件按平台指向 `../../deps/<platform>`。

## 设计文档

- [architecture.md](docs/architecture.md) — 架构设计：技术分层、读写路径、数据流
- [backend.md](docs/backend.md) — 后端开发指南：Go 包、DuckDB 表、REST API
- [frontend.md](docs/frontend.md) — 前端开发指南：Vue 组件、store、hash 路由
- [interaction.md](docs/interaction.md) — 交互设计：层级模型、下钻/聚焦/过滤
- [roadmap.md](docs/roadmap.md) — 基线（已达成）与候选路标（待讨论）
