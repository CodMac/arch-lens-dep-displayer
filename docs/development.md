# 开发指南

> 面向维护者。本文给出开发流程、硬性约束与常见坑。架构与接口见 [architecture.md](architecture.md)，表结构见 [schema.md](schema.md)。

## 1. 环境与构建

| 依赖 | 说明 |
|---|---|
| Go 1.25+ | 后端 |
| Node.js 18+ | 前端构建（首次启动会自动 `npm install` + `npm run build`） |
| DuckDB 动态库 | `deps/windows_amd64/duckdb.dll` 或 `deps/linux_amd64/libduckdb.so` |
| analyzer 源码 | `go.mod` 用 `replace ... => ../arch-lens-dep-analyer`，需同级目录存在 |

```bash
# 后端
go build ./...
go vet ./...
gofmt -l ./cmd ./internal        # 应无输出

# 前端（严格类型门禁）
cd front && npm install
npx vue-tsc -b && npx vite build
```

```bash
# 本地运行
go build -o .bin/arch-lens-dep-displayer.exe ./cmd/displayer
./.bin/arch-lens-dep-displayer.exe -listen :8080 -browser=false

# 前端热更新（/api 代理到 :8090，可用 VITE_API_TARGET 覆盖）
cd front && npm run dev
```

Windows 下运行需把 `duckdb.dll` 放在 exe 同目录（或把 `deps/windows_amd64` 加入 `PATH`）；Linux 用 `LD_LIBRARY_PATH`。

### 1.1 验证方式

改动 pipeline / 预计算后，用真实源码目录跑一次完整 5 步分析，确认关键指标未漂移（元素/关系数、类型级边对数、方法级边对数、环组数、高扇入/扇出命中数）。各语言的验收基线见 [languages/](languages/)。

## 2. 硬性约束

### 2.1 分层与依赖方向

```
api → { api/service, api/dto, registry }
api/service → { api/dto, db/repository, pipeline/preprocess, registry }
registry → { db, pipeline }
pipeline → { db, db/repository, pipeline/ingest, pipeline/preprocess }
pipeline/{ingest,preprocess} → db/repository
db → duckdbthin
```

- **不得出现反向依赖或环**。`db/repository` 不 import 任何上层包。
- `cmd/displayer` 是纯入口，不含分析逻辑。

### 2.2 SQL 全部收敛在 `db/repository`

- `pipeline`、`ingest`、`preprocess` **不得直接操作 `*sql.DB`**，必须通过 `repository.Of(db)`。
- 读路径用 `repository.New(func() *sql.DB)`（连接随激活源动态取）。
- 方法按**被写/被查的主表**分文件；JOIN/聚合放驱动表文件；单表查询放该表文件。
- 共享 SQL 辅助函数放 `sqlutil.go`，不要散落到各文件。
- repository 是纯数据访问层，不掺业务判断。

### 2.3 预计算层不得修补解析缺陷

analyzer 的解析问题（端点悬空、默认包空归属、跨文件 static 丢弃、C++ 重开 namespace 悬空 `type_qn` 等）只能**记录并抛回 analyzer**，`ingest` / `preprocess` 不得做 QN/kind 改写或语义兜底。

**为什么**：补丁会把「数据源错误」固化成「派生层约定」，analyzer 修复后补丁变成二次污染。唯一的例外是查询期展示兜底（如默认包用 `COALESCE(NULLIF(namespace_qn,''),'(default)')` 参与建树），且不改写 `elements`。

### 2.4 归属字段由解析器给，不在后端推导

`namespace_qn` / `enclosing_qn` / `type_qn` 由 analyzer 直接输出，ingest 原样落列：

| 字段 | 语义 | 用途 |
|---|---|---|
| `namespace_qn` | 命名空间容器（Java package / C++ namespace / module） | L1 建树 |
| `enclosing_qn` | 直接词法外层（类 / 方法 / lambda / 匿名类 / 块） | L2 建树、元素树 |
| `type_qn` | 最外层具名类型（宿主类型） | 嵌套类归并、类型级边、环投影 |

**禁止**从 QN 字符串反推归属（切分隔符、按大小写猜类型）。`qn_separator` 只用于展示与兜底。

### 2.5 命名约定

**唯一事实来源**：本节。其他文档与代码冲突时以本节为准；改命名先改本节。

核心术语：

| 概念 | 唯一叫法 | 说明 |
|---|---|---|
| 层级角色 | `namespace` / `type` / `member` | `dimension_levels.role`，不随语言变 |
| 命名空间容器 | `namespace_qn` | Java `PACKAGE` / C++ `NAMESPACE` / Python·Rust `MODULE` |
| 直接词法外层 | `enclosing_qn` | 类 / 方法 / lambda / 匿名类 / 作用域块 |
| 最外层具名类型 | `type_qn` | 嵌套类归并、类型级边、环投影的唯一依据 |
| 元素种类 | `kind` | `ElementKind` 真实值（`CLASS` / `STRUCT` / `METHOD` …） |
| 关系种类 | `relation_type` | `CALL` / `USE` / `IMPL_LINK` …；**不叫 `type`** |
| 违规维度 | `metric` | `fanin` / `fanout`；**不叫 `kind`、不叫 `type`** |

禁用规则：

| 禁止 | 原因 |
|---|---|
| `class_edge` / `class_fan` / `violation_class` / `class_qn` / `classCount` / `hostClass` | 维度是 **type**，不是 class（Java type 层含 CLASS/INTERFACE/ENUM/ANNOTATION；C++ 含 STRUCT/UNION/TYPE） |
| 用 `type` 表示「关系种类」 | 与 `type_qn` 并存有歧义；新表一律 `relation_type` |
| 一列两义 | `type_fan.kind` = ElementKind；`violation_hit.metric` = 违规维度 |

保留项：`CLASS` / `ANONYMOUS_CLASS` / `STRUCT` 等真实枚举值；Vue 模板 `class="..."` 与 CSS 类名；`className` DOM 属性。

重命名映射（历史，供查证）：

| 旧 | 新 |
|---|---|
| `class_edge` / `class_fan` / `violation_class` | `type_edge` / `type_fan` / `violation_hit` |
| `class_cycle` | `cycle_group` + `cycle_member` + `cycle_edge` |
| `class_qn` / `class_count` / `class_total` | `type_qn` / `type_count` / `type_element_total` |
| `subtree_class_count` / `direct_class_count` | `subtree_type_count` / `direct_type_count` |
| `fan_in_classes` / `fan_out_classes` | `fan_in_types` / `fan_out_types` |
| `violation_class.kind`（存 fanin/fanout） | `violation_hit.metric` |
| `cycle_types` | `cycle_relation_types` |
| `GET /class-edges` / `/classes` / `granularity=class` | `/type-edges` / `/types` / `granularity=type` |
| `hostClass` / `hostType`（前端） | 删除，改读 `type_qn` |
| `ClassEdge` / `fetchClassEdges` / `classCount` | `TypeEdge` / `fetchTypeEdges` / `typeCount` |

中文文案：类级边 → 类型级边；类数 → 类型数；类总数 → 类型元素总数（与「宿主类型数」区分）；高扇入类 → 高扇入类型。

校验（应为 0）：

```bash
grep -rn "class_edge\|class_fan\|violation_class\|class_qn\|class_count\|class_total" docs/ internal/ front/src/
grep -rn "classCount" docs/ internal/ front/src/
grep -rn "granularity.*'class'" front/src/
```

### 2.6 语法糖：两个正交概念

| 概念 | 字段 | 含义 | 前端用途 |
|---|---|---|---|
| 编译器隐式生成 | `elements.is_sugar`（analyzer 语义） | 源码里没有对应代码（`@Data` 的 getter、默认构造器、`enum.values()`） | L3 元素树徽标 |
| 源码语法结构 | `elements.is_impl_detail`（后端按 kind 派生） | `kind ∈ LAMBDA / ANONYMOUS_CLASS / METHOD_REF / SCOPE_BLOCK` | L2「隐藏实现细节」 |

两者正交（lambda 是源码结构、Lombok getter 是隐式生成）。**禁止**在前端用 QN 正则判定语法糖。

### 2.7 违规计算只在服务端

- `type_fan` 是与阈值无关的原始指标；`violation_config` 存口径；`violation_hit` / `cycle_*` / `pkg_rollup` 是派生结果。
- 前端只做类型/常量来源与包级向上汇聚，不本地算 SCC、不本地判阈值。
- `PUT /violations/config` 是唯一的「请求期写派生表」例外：只重建 `violation_hit` / `cycle_*` / `pkg_rollup`，`type_edge` / `type_fan` 不动。
- **发生点在类型级**：扇入/扇出/环都在 `type_qn` 归一化后判定，包层只做子树向上汇聚。

### 2.8 表结构与 schema 变更

- 改 `internal/db/schema.go` 的 DDL 后**必须递增 `SchemaVersion`**（版本不符会删库重建，不做迁移）。
- 同步更新 `db/repository/` 的读写方法与 [schema.md](schema.md)。
- 关键取舍（不要随意改）：
  - `elements` 主键 `(qn, kind)`：C++ 声明/定义共享 QN。
  - `relations` 落 `source_kind` / `target_kind`：C++ `IMPL_LINK` 两端 QN 相同。
  - `method_edge` 是 VIEW 不是表（由 `call_site` 聚合）。
  - `type_edge` 同时存 `edge_count`（加权）与 `site_count`（去重引用点）。
  - `cycle_edge` 存 `site_count` 而非 path/line（与 `call_site` 有 QN 层级断点）。
  - `elements.extra` 存 JSON 不拆列（Mores 键是语言特化的）。
  - `metrics.type_total`（宿主类型数）与 `type_element_total`（类型元素数）是两个口径，不要合并。

## 3. DuckDB 驱动的两个坑

| 坑 | 现象 | 规避 |
|---|---|---|
| 单连接 | 游标未关就再次查询会**永久死锁**（`SetMaxOpenConns(1)`） | 每个 `rows` 必须 `defer rows.Close()`；不要在遍历中发起新查询 |
| `VARCHAR[]` 绑定/读回 | 驱动不支持切片参数；读回是 `[CALL, USE]` 字符串 | 写入一律用 SQL `array_agg`（不经 Go 传数组）；读取统一用 `CAST(types AS VARCHAR)` 后解析 |

## 4. 常见开发任务

### 4.1 加新语言维度

1. 新建 `config/dimensions/{lang}.yaml`（参考 `java.yaml`：`hierarchy` + `relation_types` + `method_kinds` + `structural_relation_types`）。
2. 用 `ARCH_LENS_DIM` 指向该文件（或放入默认查找路径）。
3. 创建数据源时 `language` 传对应语言。
4. 补 `docs/languages/{lang}.md`，说明关系类型全集与端点 kind 矩阵；并在 [languages/README.md](languages/README.md) 登记。

### 4.2 加新 API 端点

1. `api/dto/` 定义请求/响应结构。
2. `api/service/` 加业务方法（调用 repository 或 registry）。
3. `api/` 加薄壳 handler（参数解析 + 调 service + 写响应）。
4. `router.go` 注册路由。
5. 按业务域放到对应模块文件（三处同名），不要新建「技术形态」文件。

### 4.3 加新查询 / 预计算

- 纯查询：在 `db/repository/` 按主表加方法，直接写 SQL。
- 新预计算：在 `preprocess` 加步骤，SQL 走 repository；同步 `clearDerived` / `RebuildDerived` 与 `SchemaVersion`。
- 前端契约：同步 `internal/api/dto` 与 `front/src/proto/types/model.ts`。

### 4.4 前端改动

- 唯一入口 `front/src/main.ts → proto/ProtoApp.vue`；不要新增第二个 HTML 入口。
- 视图只消费 `api/proto.ts` 的返回；不要在视图里做 BFS / SCC / QN 解析。
- 新增路由需同时改 `proto/router.ts` 的 `encodeRoute` / `decodeRoute`。

## 5. 已知取舍

这些是**有意接受**的现状，改动前需先评估影响面：

| 项 | 现状 | 处理 |
|---|---|---|
| 默认包无归属 | 无 `package` 声明的元素 `namespace_qn` 为空 | 后端不改写，查询期 `COALESCE` 兜底展示；已抛 analyzer |
| 跨文件同名 `static` 被丢弃 | 第二个元素消失，调用解析到第一个 | 本轮接受；analyzer 修复后主键可升级为 `(qn, kind, path)` |
| C++ 重开 namespace 的 `type_qn` 悬空 | 少量 `type_qn` 指向不存在的元素 | 只计数告警，`element_node` 按回退链挂到 namespace；已抛 analyzer |
| 外部依赖不可见 | analyzer 已过滤外部 IMPORT/THROW | 本轮接受；将来另建 `external_edge`，不混入 `type_edge` |
| 同一位置多条关系 | 如 `CREATE + THROW` 同点 | `dedupe_same_site` 默认 true，阈值按去重引用点判定；加权值仍在 `type_edge` |
| 重跑 pipeline 丢标注 | 删库重建 | 已知取舍；标注是唯一与「分析结果」无关的用户数据 |
| 单连接串行 | DuckDB 单连接 | 重查询（`/fan`）串行化；必要时加只读连接池或结果缓存 |
