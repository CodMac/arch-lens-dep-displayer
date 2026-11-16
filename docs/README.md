# 文档索引

按「设计者视角」组织：4 份总体文档 + 语言特性子目录。

## 总体文档

| 文档 | 回答的问题 |
|---|---|
| [architecture.md](architecture.md) | 项目架构是怎样的？技术选型有哪些？数据怎么流动？对外有哪些接口？前端有哪些视图？ |
| [development.md](development.md) | 如何构建/运行/验证？有哪些硬性约束（分层、SQL 收敛、命名、归属字段、违规口径）？常见坑与已知取舍？ |
| [schema.md](schema.md) | DuckDB 三层表结构逐表详解 + 同一真实案例的每表数据 |
| [roadmap.md](roadmap.md) | 已达成基线是什么？候选方向有哪些？ |

## 语言特性文档

| 文档 | 内容 |
|---|---|
| [languages/README.md](languages/README.md) | 语言特性文档索引与新增语言步骤 |
| [languages/java.md](languages/java.md) | Java 口径：维度配置、关系类型全集、边/扇入扇出/违规/环计算、验收基线 |
| [languages/cpp.md](languages/cpp.md) | C++ 口径：同上，含 `IMPL_LINK`、声明/定义共享 QN、`::` 分隔符等特化 |
