# arch-lens-dep-displayer

代码依赖可视化工具。一条命令启动服务，在浏览器中选择源码目录，自动完成依赖分析，并逐层查看包 / 类 / 成员之间的依赖关系。

## 快速开始

```bash
# 1. 构建（首次运行会自动 npm install + 构建前端，需本机有 Node.js）
go build -o .bin/arch-lens-dep-displayer.exe ./cmd/displayer

# 2. 启动（默认 http://localhost:8080，自动打开浏览器）
./.bin/arch-lens-dep-displayer.exe
```

## 在页面上使用

1. **选择源码目录**（首页）：输入源码目录的绝对路径，或点「浏览…」逐级选择，点「开始分析」。最近打开过的目录会记住，下次可直接点选。
2. **等待分析**：页面实时显示 5 步进度 —— 扫描 → 依赖分析（含导出）→ 导入 DuckDB → 结果预处理 → 就绪。中断后再次启动会从失败步骤续跑。
3. **查看依赖**：
   - **全景**：顶层包气泡图 + 包间依赖边；可切换 DSM 矩阵，查看缺陷概览（高扇入 / 高扇出 / 循环依赖）。
   - **影响面**：双击包下钻，或从节点详情进入蝴蝶图，查看类 / 包 / 方法级的扇入扇出。
   - **源码**：查看类或调用点的源码，自动定位并高亮对应行。
4. **切换数据源**：任意分析页右上角「切换数据源」可回到首页；已分析过的目录会保留，无需重新分析。

## 启动参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-listen` | `:8080` | HTTP 监听地址 |
| `-out` | `./.arch_lens/sources` | 数据源工作区根目录（每个源码目录一个子目录） |
| `-browser` | `true` | 启动后是否自动打开浏览器 |

```bash
# 示例：换端口、换工作区、不自动开浏览器
./.bin/arch-lens-dep-displayer.exe -listen :9090 -out D:/lens-data -browser=false
```

## 平台依赖：DuckDB 动态库

程序通过 cgo 直接调用 DuckDB C API，需要随程序提供对应平台的动态库：

| 平台 | 目录 | 库文件 |
|---|---|---|
| Windows amd64 | `deps/windows_amd64/` | `duckdb.dll` |
| Linux amd64 | `deps/linux_amd64/` | `libduckdb.so` |

- Windows：把 `duckdb.dll` 放在可执行文件同目录。
- Linux：设置 `LD_LIBRARY_PATH` 指向 `deps/linux_amd64`。

## 文档

- [docs/README.md](docs/README.md) — 文档索引（总体文档 + 语言特性文档）
