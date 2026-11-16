# 路标（Roadmap）

> 更新：2026-11-16 ｜ 本文只记录「已达成基线」与「候选方向」；候选方向均标 ⚪ 待讨论，未排期。

## 0. 当前基线（已达成 ✅）

产品形态已定型为**单一命令启动器 + 多数据源**：`arch-lens-dep-displayer [-listen :8080] [-out ./.arch_lens/sources] [-browser]`，启动即托管前端 + REST API。用户在页面指定源码目录，后台完成 5 步分析（scan → analyze → ingest → preprocess → activate），支持多数据源切换与断点续跑。

### 功能

| 主题 | 内容 | 状态 |
|---|---|---|
| 启动器 | 唯一命令入口，托管前端 + REST API，自动开浏览器 | ✅ |
| 数据源 | 多数据源工作区、激活切换、断点续跑、进度轮询 | ✅ |
| 全景 | 包树气泡 + 包间聚合边 + DSM 矩阵 + 多主框下钻 | ✅ |
| 缺陷 | 高扇入 / 高扇出 / 跨包环链路高亮 + 可调口径（服务端重算） | ✅ |
| 影响面 | 包 / 类 / 方法三档蝴蝶图 + 节点与边详情 + 调用点展开 | ✅ |
| 源码 | 元素树 + 行高亮 + 双节点预览 | ✅ |
| URL 记忆 | 视图与下钻状态序列化到 hash | ✅ |

### 架构

| 项 | 说明 | 状态 |
|---|---|---|
| 分层 | `api（router→handler→service→dto）→ {registry, db/repository}`；写路径 `pipeline → {ingest, preprocess} → repository → db` | ✅ |
| 读写路径分离 | repository 常驻读模型（`func() *sql.DB` 动态取连接）；ingest/preprocess 一次性写路径 | ✅ |
| SQL 收敛 | 全部 SQL 在 `db/repository`（按主表分文件），pipeline 不直接操作 db | ✅ |
| 维度配置驱动 | `config/dimensions/*.yaml`（role/kinds/qn_separator/method_kinds/structural_relation_types），`ARCH_LENS_DIM` 可覆盖 | ✅ |
| 预计算换查询 | 原始层 + 维度层 + 预计算层（type_edge / call_site / type_fan / violation_* / cycle_* / pkg_rollup / metrics） | ✅ |
| 多语言 | Java + C++ 口径已落地，归属字段由 analyzer 输出 | ✅ |
| 前端 | 单入口（proto L1→L4），无 mock、无死页面 | ✅ |

## 1. 候选方向（⚪ 待讨论）

| # | 方向 | 一句话说明 | 状态 |
|---|---|---|---|
| C1 | 更多语言维度 | 新增 Go / Python / Rust 维度配置与口径文档 | ⚪ 待讨论 |
| C2 | 增量导入 | 基于文件变更时间 / sha1 的增量分析，替代全量重建 | ⚪ 待讨论 |
| C3 | 两版本对比 | 同一项目两个版本的差异视图 | ⚪ 待讨论 |
| C4 | 大图分块加载 | 大数据量图分块加载 / 实时推送，替代整层查询 | ⚪ 待讨论 |
| C5 | 标注增强 | 标注标签、分组、导出；标注在重跑 pipeline 后保留 | ⚪ 待讨论 |
| C6 | 结果导出 | 图 / 矩阵 / 环组导出为 JSON / SVG / CSV | ⚪ 待讨论 |
| C7 | 外部依赖可见 | 另建 `external_edge` 展示三方库依赖（当前 analyzer 已过滤外部端点） | ⚪ 待讨论 |
