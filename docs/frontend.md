# 前端开发指南

> 面向维护者。前端（`front/`）是同一进程内的展示部分，对外不区分前后端（见 [architecture.md](architecture.md)）。本文描述 Vue 组件结构、状态管理、hash 路由与后端契约。

## 1. 技术栈

- Vue3（`<script setup>` Composition API）+ Vite + TypeScript（vue-tsc 严格模式构建）
- Pinia（状态管理）
- AntV G6 v5（图渲染）
- Axios（REST 调用）
- 无 vue-router：视图切换由 App.vue 的 `mode` + hash 承载

## 2. 目录结构

```
front/src/
├── main.ts              # 入口（createApp + Pinia）
├── App.vue              # 根：home/overview/level 三视图 + hash 同步
├── api/                 # REST 封装（axios）
│   ├── client.ts        #   get/post/put/del 泛型 helper
│   ├── sources.ts       #   数据源 + fs 目录浏览
│   ├── dimensions.ts    #   维度列表
│   ├── nodes.ts         #   图数据：top/children/graph/fan/source/overview
│   ├── metrics.ts       #   scc/metrics/search/meta/topby
│   └── annotations.ts   #   标注/收藏
├── stores/              # Pinia store
│   ├── source.ts        #   数据源列表 + 激活源
│   ├── dimension.ts     #   维度列表 + 当前维度
│   ├── layerStack.ts    #   下钻栈（含 ROOT + hydrate）
│   ├── graphData.ts     #   当前图数据 + 类型过滤 + fan 叠加
│   ├── pathStore.ts     #   选点找路径（A→B）
│   ├── annotations.ts   #   标注列表（收藏集）
├── components/          # 组件
│   ├── GraphCanvas.vue  #   G6 封装（渲染/交互/右键菜单/星标）
│   ├── Breadcrumb.vue   #   下钻面包屑
│   ├── SearchBar.vue    #   全局搜索
│   ├── FilterBar.vue    #   关系类型过滤
│   ├── DataInfoPanel.vue#   数据可信度信息栏（meta）
│   ├── MetricCard.vue   #   指标卡（TOP 列表）
│   ├── StarPanel.vue    #   收藏侧栏
│   ├── SourcePanel.vue  #   源码定位面板
├── utils/hashState.ts   # hash ⇄ 状态编解码
├── types/index.ts       # 与后端契约对齐的 TS 类型
└── views/
    ├── SourceHomeView.vue  # 数据源首页（新建/选择）
    ├── OverviewView.vue    # 仓库全景（level=1 包）
    └── LevelGraphView.vue  # 层级图（L1+ 下钻）
```

## 3. 状态流

### 3.1 视图模式

App.vue 的 `mode`：`home`（无激活源引导创建）→ `overview`（仓库全景）→ `level`（层级图）。`effectiveMode` 计算属性保证无激活源时始终停留 home。

### 3.2 下钻栈

`layerStack.stack` 以 ROOT 开头，每层存 `{level, qn, label, kind}`。下钻 = push，返回 = pop，`hydrate()` 支持从 hash 恢复整栈。

### 3.3 数据流（一次下钻）

```
OverviewView ──点包──► GraphCanvas emit node-action('drill')
  └─► layerStack.push({level:1, qn}) ──► graphDataStore.loadChildren(qn)
        └─► api.nodes.fetchChildren → GET /nodes/{qn}/children
              └─► G6 渲染子节点
```

## 4. URL 记忆（hash 路由）

格式：`#/d/{sid}/{mode}/{level}:{qn}/...`

```
#/d/0796aef8cbd0/overview
#/d/0796aef8cbd0/level/1:com.ysj/2:com.ysj.tinySpring.beans
```

- `encode(state)` / `decode(hash)` 在 `utils/hashState.ts`（qn 经 `encodeURIComponent`）。
- App.vue 监听 `hashchange`；状态变化（mode/stack/activeSid）防抖写回 hash。
- 双重防环：写回前比较当前 hash，应用前比较当前状态。

## 5. 与后端契约

TS 类型（`types/index.ts`）与后端 `internal/api/dto` 一一对应：

| 后端 dto | 前端类型 | 说明 |
|---|---|---|
| `Node` | `GraphNode` | id/label/kind/size |
| `Edge` | `GraphEdge` | source/target/type/types/count |
| `Dimension`/`Level` | `Dimension`/`DimensionLevel` | 维度 + 层级 |
| `SourceLoc` | `SourceInfo` | qn/path/start_line/name |
| `SccComp` | `SccComponent` | id/members |
| `MetaResp` | `MetaInfo` | 可信度信息 |
| `TopItem` | `TopItem` | qn/label/kind/value |
| `Annotation` | `Annotation` | 收藏/备注 |
| `Source` | `Source` | 数据源 + 步骤 |

> 变更契约时需同步两侧：后端 `internal/api/dto` 与前端 `front/src/types/index.ts`。

## 6. GraphCanvas（G6 封装要点）

- 节点样式：size → 半径映射；收藏节点金色描边 + `★` 前缀 + 淡黄底。
- 边宽：`count` log 映射（包间聚合边宽=依赖条数）。
- 右键菜单：下钻/扇入/扇出/查看源码/收藏，经 `node-action` emit 分发。
- 环高亮：SCC 成员红框 + tooltip 环号；高扇入节点紫描边。
- 选点找路径：`pathStore` 起点→终点，前端 BFS 求最短路径，蓝色高亮。

## 7. 构建与开发

```bash
cd front
npm install
npm run dev        # Vite 开发服务器（HMR）
npm run build      # vue-tsc -b && vite build（严格类型门禁）
```
