<script setup lang="ts">
// 通用层级图视图：L1/L2/L3 复用，由维度配置 + 层级栈驱动。
// D3 能力：扇入/扇出 N 层展开（右键菜单 + 深度选择）、A→B 依赖路径（选点 BFS + 高亮）。
import { ref, watch, computed, onMounted } from 'vue'
import Breadcrumb from '../components/Breadcrumb.vue'
import FilterBar from '../components/FilterBar.vue'
import MetricCard from '../components/MetricCard.vue'
import GraphCanvas from '../components/GraphCanvas.vue'
import SearchBar from '../components/SearchBar.vue'
import DataInfoPanel from '../components/DataInfoPanel.vue'
import SourcePanel from '../components/SourcePanel.vue'
import StarPanel from '../components/StarPanel.vue'
import { useDimensionStore } from '../stores/dimension'
import { useLayerStackStore } from '../stores/layerStack'
import { useGraphDataStore } from '../stores/graphData'
import { usePathStore } from '../stores/pathStore'
import { useAnnotationsStore } from '../stores/annotations'
import { fetchTop, fetchGraph, fetchChildren, fetchFanin, fetchFanout, fetchFanoutAdj } from '../api/nodes'
import { fetchMetrics, fetchScc } from '../api/metrics'
import type { GraphData, GraphNode, GraphEdge } from '../types'

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()
const graphStore = useGraphDataStore()
const pathStore = usePathStore()
const annotations = useAnnotationsStore()

const canvasRef = ref<InstanceType<typeof GraphCanvas> | null>(null)
const sourcePanelRef = ref<InstanceType<typeof SourcePanel> | null>(null)
// 扇入/扇出深度选择（右键触发后弹出）。
const fanPrompt = ref<{ id: string; mode: 'in' | 'out' } | null>(null)
const fanDepthInput = ref(2)

const metrics = ref<Record<string, number>>({})
// 环成员 QN → 环 ID 列表（一个节点可属多个环）。
const cycleOf = ref<Record<string, number[]>>({})
const nodeCount = computed(() => graphStore.data.nodes.length)
const edgeCount = computed(() => graphStore.visibleEdges.length)
const cycleCount = computed(() => Object.keys(cycleOf.value).length > 0 ? new Set(Object.values(cycleOf.value).flat()).size : 0)

// 高扇入节点：当前图中入边数 ≥ 阈值的节点（前端推导，无需新接口）。
// 阈值 = max(3, 入边数的 p90)，保证只在真正密集的节点上打标。
const highFaninSet = computed<Set<string>>(() => {
  const inDeg = new Map<string, number>()
  for (const e of graphStore.visibleEdges) {
    if (e.type === 'CONTAIN') continue
    inDeg.set(e.target, (inDeg.get(e.target) ?? 0) + 1)
  }
  const vals = [...inDeg.values()].sort((a, b) => b - a)
  if (!vals.length) return new Set()
  const threshold = Math.max(3, vals[Math.min(vals.length - 1, Math.floor(vals.length * 0.1))])
  return new Set([...inDeg.entries()].filter(([, v]) => v >= threshold).map(([k]) => k))
})

// 当前焦点节点在栈中的层级（栈顶）。
const focusLevel = computed(() => {
  const cur = layerStack.current
  return cur.qn === '' ? 1 : cur.level + 1
})

// 下钻：点击节点 → 取子级 → 若为空则切到 graph 视图。锚点（外部灰显节点）不响应。
function drill(id: string): void {
  const node = graphStore.data.nodes.find((n) => n.id === id)
  if (!node || (node as GraphNode & { anchor?: boolean }).anchor) return
  // 聚焦分析中下钻会清空叠加，回到新层的干净视图。
  pathStore.clear()
  graphStore.clearFan()
  const level = nodeLevel(node)
  layerStack.push({ level, qn: node.id, label: node.label, kind: node.kind })
}

// ---- D3-1 扇入/扇出 N 层展开 ----

// 右键菜单动作分发。
function onNodeAction(id: string, action: 'drill' | 'fanin' | 'fanout' | 'source' | 'star' | 'unstar'): void {
  if (action === 'drill') {
    drill(id)
    return
  }
  if (action === 'source') {
    void sourcePanelRef.value?.openFor(id)
    return
  }
  if (action === 'star' || action === 'unstar') {
    const dimId = dimensionStore.currentId
    if (dimId) void annotations.toggleStar(dimId, id)
    return
  }
  fanPrompt.value = { id, mode: action === 'fanin' ? 'in' : 'out' }
  fanDepthInput.value = 2
}

// 确认扇入/扇出深度展开。
async function applyFan(): Promise<void> {
  const p = fanPrompt.value
  if (!p) return
  const dimId = dimensionStore.currentId
  if (!dimId) return
  fanPrompt.value = null
  graphStore.fanLoading = true
  try {
    const fan = p.mode === 'in'
      ? await fetchFanin(dimId, p.id, fanDepthInput.value)
      : await fetchFanout(dimId, p.id, fanDepthInput.value)
    graphStore.mergeFan(fan, p.id, p.mode, fanDepthInput.value)
  } catch (e) {
    graphStore.error = String(e)
  } finally {
    graphStore.fanLoading = false
  }
}

// ---- D3-2 A→B 依赖路径 ----

// 选点模式：单击节点进入选点。
function onNodeSelect(id: string): void {
  const node = graphStore.data.nodes.find((n) => n.id === id)
  if (!node) return
  if (!pathStore.from) {
    pathStore.setStart(node)
  } else if (!pathStore.to) {
    pathStore.setEnd(node)
    void findPath()
  } else {
    // 已求完路径后再点：重新设为起点。
    pathStore.setStart(node)
  }
  // 切回普通模式（点击节点恢复下钻）。
  canvasRef.value?.setSelectMode(false)
}

// BFS：以起点为根逐层 /fanout?depth=1 求前向可达，找终点最近路径。
// 图规模可控（每次只取邻接子图），最多 maxSteps 层防止爆炸。
async function findPath(): Promise<void> {
  const dimId = dimensionStore.currentId
  const from = pathStore.from
  const to = pathStore.to
  if (!dimId || !from || !to) return
  pathStore.searching = true
  pathStore.error = ''
  const maxSteps = 6
  try {
    // BFS 队列：{ qn, prev（指向前驱条目）, edge（到达本节点的边） }。
    interface BFSEntry { qn: string; prev: BFSEntry | null; edge: GraphEdge | null }
    const visited = new Set<string>([from.qn])
    let frontier: BFSEntry[] = [{ qn: from.qn, prev: null, edge: null }]
    let found: BFSEntry | null = null
    for (let step = 0; step < maxSteps && !found; step++) {
      const next: BFSEntry[] = []
      for (const cur of frontier) {
        if (cur.qn === to.qn) {
          found = cur
          break
        }
        const adj = await fetchFanoutAdj(dimId, cur.qn)
        for (const e of adj.edges) {
          if (e.target === cur.qn) continue // 排除自环
          if (visited.has(e.target)) continue
          visited.add(e.target)
          next.push({ qn: e.target, prev: cur, edge: e })
        }
      }
      if (found) break
      frontier = next
    }
    if (!found) {
      pathStore.error = `未找到 ${from.label} → ${to.label} 的路径（已探 ${maxSteps} 层）`
      return
    }
    // 从终点回溯到起点，倒序展开。
    const nodeSeq: GraphNode[] = []
    const edgeSeq: GraphEdge[] = []
    const chain: BFSEntry[] = []
    for (let c: BFSEntry | null = found; c; c = c.prev) chain.push(c)
    chain.reverse()
    for (const c of chain) {
      nodeSeq.push({ id: c.qn, label: lastSeg(c.qn), kind: '', size: 0 })
      if (c.edge) edgeSeq.push(c.edge)
    }
    pathStore.setPath(nodeSeq, edgeSeq)
    graphStore.mergePath(nodeSeq, edgeSeq)
  } catch (e) {
    pathStore.error = String(e)
  } finally {
    pathStore.searching = false
  }
}

// 清空路径分析状态。
function clearPath(): void {
  pathStore.clear()
  // 需要把叠加进 data 的锚点节点/边移除：重新 load 当前层。
  void load()
}

function lastSeg(qn: string): string {
  const i = qn.lastIndexOf('.')
  return i >= 0 ? qn.slice(i + 1) : qn
}

// 节点所属层级：包=1 类=2 成员=3（对齐 dimension levels）。
function nodeLevel(n: GraphNode): number {
  const ls = dimensionStore.levels
  for (const lv of ls) {
    if (lv.kind === n.kind || lv.parent_of.includes(n.kind)) return lv.level
  }
  return ls[0]?.level ?? 1
}

async function load(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  const cur = layerStack.current
  graphStore.loading = true
  try {
    let data: GraphData
    if (cur.qn === '') {
      // 顶层：包视图。
      data = await fetchTop(dimId, focusLevel.value)
    } else {
      // 下钻层：先取子级（本层），再取该节点 graph（锚点边）。
      const children = await fetchChildren(dimId, cur.qn)
      const graph = await fetchGraph(dimId, cur.qn)
      data = mergeDrill(children.nodes, graph)
    }
    graphStore.setData(data)
    await loadScc(dimId)
  } catch (e) {
    graphStore.error = String(e)
  } finally {
    graphStore.loading = false
  }
}

// 下钻合并：本层子节点 + 焦点邻接边（外部锚点带 anchor 标记，GraphCanvas 灰显）。
function mergeDrill(children: GraphNode[], graph: GraphData): GraphData {
  const nodes = [...children]
  const seen = new Set(nodes.map((n) => n.id))
  // 补充邻接节点（锚点，非子级）：保留真实 kind，附 anchor 标记。
  for (const n of graph.nodes) {
    if (!seen.has(n.id)) {
      nodes.push({ ...n, kind: n.kind, anchor: true } as GraphNode & { anchor: boolean })
    }
  }
  return { nodes, edges: graph.edges }
}

async function loadMetrics(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  try {
    metrics.value = await fetchMetrics(dimId)
  } catch {
    metrics.value = {}
  }
}

async function loadScc(dimId: number): Promise<void> {
  try {
    const comps = await fetchScc(dimId)
    // 环成员 QN → 环 ID 映射（高亮 + tooltip 环归属）。
    const m: Record<string, number[]> = {}
    for (const c of comps) {
      for (const mem of c.members) {
        ;(m[mem] ??= []).push(c.id)
      }
    }
    cycleOf.value = m
  } catch {
    cycleOf.value = {}
  }
}

// 切维度 / 层级栈变化 → 重新加载。
watch(
  () => [dimensionStore.currentId, layerStack.stack.map((s) => s.qn).join('>')],
  () => void load(),
)

// 切维度加载标注（收藏列表按维度隔离）。
watch(
  () => dimensionStore.currentId,
  (id) => {
    if (id != null) void annotations.load(id)
  },
)

onMounted(() => {
  void loadMetrics()
  if (dimensionStore.currentId != null) void annotations.load(dimensionStore.currentId)
})
</script>

<template>
  <div class="level-view">
    <header class="toolbar">
      <span class="title">
        {{ dimensionStore.current?.name ?? '…' }} 依赖图
        <span v-if="dimensionStore.current" class="lang">[{{ dimensionStore.current.language }}]</span>
      </span>
      <div class="toolbar-right">
        <SearchBar />
        <select
          v-model.number="dimensionStore.currentId"
          class="dim-select"
          @change="layerStack.reset()"
        >
          <option v-for="d in dimensionStore.dimensions" :key="d.id" :value="d.id">
            {{ d.name }} ({{ d.language }})
          </option>
        </select>
      </div>
    </header>
    <Breadcrumb />
    <FilterBar />

    <section class="metrics">
      <MetricCard label="节点" :value="nodeCount" color="#5B8FF9" />
      <MetricCard label="边" :value="edgeCount" color="#5AD8A6" />
      <MetricCard label="环数" :value="cycleCount" color="#F6BD16" />
      <MetricCard label="包总数" :value="metrics.package_total ?? 0" />
      <MetricCard label="类总数" :value="metrics.class_total ?? 0" />
      <MetricCard label="关系总数" :value="metrics.relation_total ?? 0" />
    </section>

    <section class="canvas-wrap">
      <div v-if="graphStore.loading" class="overlay">加载中…</div>
      <div v-else-if="graphStore.error" class="overlay error">{{ graphStore.error }}</div>
      <div v-if="graphStore.fanLoading" class="overlay">展开扇入/扇出中…</div>
      <div v-if="pathStore.searching" class="overlay">搜索路径中…</div>
      <GraphCanvas
        ref="canvasRef"
        :nodes="graphStore.data.nodes"
        :edges="graphStore.visibleEdges"
        :kind-colors="dimensionStore.kindColors"
        :cycle-of="cycleOf"
        :high-fanin-nodes="highFaninSet"
        :path-highlight="pathStore.highlight"
        :starred-nodes="annotations.starredSet"
        @node-click="drill"
        @node-action="onNodeAction"
        @node-select="onNodeSelect"
      />

      <!-- D3-1 扇入/扇出深度选择弹层 -->
      <div v-if="fanPrompt" class="fan-prompt">
        <span class="fp-title">{{ fanPrompt.mode === 'in' ? '扇入' : '扇出' }}（{{ fanPrompt.id }}）</span>
        <label>深度
          <input v-model.number="fanDepthInput" type="number" min="1" max="6" />
        </label>
        <div class="fp-actions">
          <button @click="fanPrompt = null">取消</button>
          <button class="primary" @click="applyFan">展开</button>
        </div>
      </div>
      <div v-if="fanPrompt" class="fan-mask" @click="fanPrompt = null" />

      <!-- D3-2 路径分析状态条 -->
      <div v-if="pathStore.from || pathStore.to" class="path-bar">
        <span class="pb-text">{{ pathStore.hint }}</span>
        <button class="pb-clear" @click="clearPath">清除</button>
      </div>
    </section>

    <!-- D4 数据信息/异常/指标排序侧栏（fixed 定位，无需布局改动） -->
    <DataInfoPanel />

    <!-- D5 源码定位面板（需求11） -->
    <SourcePanel ref="sourcePanelRef" />

    <!-- D6 收藏侧栏（需求14） -->
    <StarPanel />
  </div>
</template>

<style scoped>
.level-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  height: 48px;
  background: #fff;
  border-bottom: 1px solid #e5e6eb;
}

.title {
  font-size: 15px;
  font-weight: 600;
}

.lang {
  color: #86909c;
  font-weight: 400;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.dim-select {
  padding: 4px 8px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  font-size: 13px;
}

.metrics {
  display: flex;
  gap: 12px;
  padding: 12px;
}

.canvas-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
  margin: 0 12px 12px;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
}

.overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #86909c;
  background: rgba(255, 255, 255, 0.7);
  z-index: 10;
}

.overlay.error {
  color: #e8684a;
}

.fan-mask {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.2);
  z-index: 20;
}

.fan-prompt {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 30;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 260px;
  padding: 16px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.15);
  font-size: 13px;
}

.fp-title {
  font-weight: 600;
  color: #1f2329;
  word-break: break-all;
}

.fan-prompt label {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #4e5969;
}

.fan-prompt input {
  width: 60px;
  padding: 4px 8px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
}

.fp-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.fp-actions button {
  padding: 5px 14px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
}

.fp-actions button.primary {
  background: #165dff;
  color: #fff;
  border-color: #165dff;
}

.path-bar {
  position: absolute;
  top: 12px;
  left: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 10px;
  font-size: 12px;
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid #c6e2ff;
  border-radius: 12px;
  color: #165dff;
}

.pb-clear {
  border: none;
  background: transparent;
  color: #e8684a;
  cursor: pointer;
  font-size: 12px;
}
</style>
