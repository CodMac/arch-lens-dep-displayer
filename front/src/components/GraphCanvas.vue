<script setup lang="ts">
// G6 v5 画布封装：渲染图 + 交互（缩放/拖拽/点击下钻）+ D2 图内洞察 + D3 聚焦分析。
// D2 能力：节点 tooltip（kind/规模/邻居数）、环成员高亮、高扇入高亮、点击聚焦。
// D3 能力：节点右键菜单（下钻 / 扇入 / 扇出）、选中节点（起点/终点）与路径高亮。
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue'
import { Graph, GraphEvent, NodeEvent } from '@antv/g6'
import type { GraphOptions, NodeData, EdgeData, IElementEvent, Element } from '@antv/g6'
import type { GraphNode, GraphEdge } from '../types'

const props = defineProps<{
  nodes: GraphNode[]
  edges: GraphEdge[]
  kindColors: Record<string, string>
  /** 环成员 QN → 环 ID 列表（高亮 + tooltip 环归属）。 */
  cycleOf?: Record<string, number[]>
  /** 高扇入节点 QN 集合（醒目样式）。 */
  highFaninNodes?: Set<string>
  /** 路径高亮：节点集合 + 边集合（QN 对）。命中节点/边加亮。 */
  pathHighlight?: { nodes?: Set<string>; edges?: Set<string> }
  /** 收藏节点 QN 集合（D6 需求14）：金色星标样式。 */
  starredNodes?: Set<string>
}>()

const emit = defineEmits<{
  (e: 'node-click', id: string): void
  /** 右键菜单动作：下钻 / 扇入 / 扇出 / 源码 / 收藏 / 取消收藏。 */
  (e: 'node-action', id: string, action: 'drill' | 'fanin' | 'fanout' | 'source' | 'star' | 'unstar'): void
  /** 点击（非下钻路径）：进入选点模式，作为路径起点/终点。 */
  (e: 'node-select', id: string): void
}>()

const container = ref<HTMLDivElement>()
let graph: Graph | null = null

// 扇入/出统计（tooltip 用）：由边推导邻居数。
const neighborCount = computed<Record<string, number>>(() => {
  const m: Record<string, number> = {}
  for (const e of props.edges) {
    m[e.source] = (m[e.source] ?? 0) + 1
    m[e.target] = (m[e.target] ?? 0) + 1
  }
  return m
})

// 布局：>60 节点用力导向，否则 dagre 分层（继承/依赖更清晰）。
function buildOptions(): GraphOptions {
  const nodeCount = props.nodes.length
  const layout = nodeCount > 60
    ? { type: 'force', preventOverlap: true, linkDistance: 80 }
    : { type: 'dagre', rankdir: 'LR', nodesep: 24, ranksep: 60 }

  const maxSize = Math.max(1, ...props.nodes.map((n) => n.size))
  const sizeOf = (size: number): number => 22 + Math.round((size / maxSize) * 34)

  const isCycle = (d: NodeData): boolean => !!props.cycleOf?.[d.id as string]
  const isHighFanin = (d: NodeData): boolean => !!props.highFaninNodes?.has(d.id as string)
  const isPathNode = (d: NodeData): boolean => !!props.pathHighlight?.nodes?.has(d.id as string)
  const isStarred = (d: NodeData): boolean => !!props.starredNodes?.has(d.id as string)

  return {
    container: container.value as HTMLElement,
    autoResize: true,
    autoFit: 'view',
    layout,
    node: {
      type: 'circle',
      style: {
        size: (d: NodeData) => sizeOf((d.data?.size as number) ?? 0),
        fill: (d: NodeData) => {
          if (isCycle(d)) return '#fde2e2' // 环成员：浅红底
          if (isPathNode(d)) return '#e6f4ff' // 路径节点：浅蓝底
          if (isStarred(d)) return '#fff7e6' // 收藏：淡黄底
          if (d.data?.anchor as boolean) return '#f2f3f5'
          return props.kindColors[(d.data?.kind as string) ?? 'UNKNOWN'] ?? '#A0A0A0'
        },
        stroke: (d: NodeData) => {
          if (isCycle(d)) return '#e8684a' // 环成员：红描边
          if (isPathNode(d)) return '#165dff' // 路径节点：蓝描边
          if (isHighFanin(d)) return '#9270CA' // 高扇入：紫描边
          if (isStarred(d)) return '#d4a017' // 收藏：金描边
          if (d.data?.anchor as boolean) return '#c9cdd4'
          return '#fff'
        },
        lineWidth: (d: NodeData) => {
          if (isCycle(d) || isHighFanin(d) || isPathNode(d)) return 3
          if (isStarred(d)) return 2.5
          return 1.5
        },
        lineDash: (d: NodeData) => ((d.data?.anchor as boolean) ? [3, 3] : []),
        labelText: (d: NodeData) => {
          const label = (d.data?.label as string) ?? String(d.id)
          return isStarred(d) ? '★ ' + label : label
        },
        labelPlacement: 'bottom',
        labelFontSize: 11,
        labelFill: (d: NodeData) => ((d.data?.anchor as boolean) ? '#86909c' : '#333'),
      },
    },
    edge: {
      type: 'line',
      style: {
        stroke: (d: EdgeData) => {
          const k = `${d.source}->${d.target}`
          if (props.pathHighlight?.edges?.has(k)) return '#165dff' // 路径边：蓝
          return d.data?.type === 'CONTAIN' ? '#d9d9d9' : '#bfc6d3'
        },
        lineWidth: (d: EdgeData) => {
          const k = `${d.source}->${d.target}`
          if (props.pathHighlight?.edges?.has(k)) return 3
          // 边宽 = 依赖条数（需求2）：count → 1~8 线性映射（log 压缩，大值不至于过粗）。
          const count = (d.data?.count as number) ?? 1
          if (count <= 1) return 1
          return Math.min(1 + Math.log2(count + 1), 8)
        },
        endArrow: true,
        strokeOpacity: 0.7,
      },
    },
    behaviors: [
      'zoom-canvas',
      'drag-canvas',
      'drag-element',
      'hover-activate',
    ],
    plugins: [
      // 节点右键菜单（D3）：下钻 / 扇入 / 扇出。
      {
        type: 'contextmenu',
        key: 'node-menu',
        trigger: 'contextmenu',
        getItems: (evt: IElementEvent) => {
          const id = (evt as unknown as { target?: { id?: string } }).target?.id as string
          if (!id) return []
          const starred = !!props.starredNodes?.has(id)
          return [
            { name: '下钻（子级）', value: 'drill' },
            { name: '扇入 …', value: 'fanin' },
            { name: '扇出 …', value: 'fanout' },
            { name: '查看源码', value: 'source' },
            starred ? { name: '取消收藏', value: 'unstar' } : { name: '★ 收藏', value: 'star' },
          ]
        },
        onClick: (value: string, _target: HTMLElement, current: Element) => {
          const id = current.id as string
          if (!id) return
          if (value === 'drill') emit('node-action', id, 'drill')
          else if (value === 'fanin') emit('node-action', id, 'fanin')
          else if (value === 'fanout') emit('node-action', id, 'fanout')
          else if (value === 'source') emit('node-action', id, 'source')
          else if (value === 'star') emit('node-action', id, 'star')
          else if (value === 'unstar') emit('node-action', id, 'unstar')
        },
      },
      // 节点 tooltip（hover 触发）。
      // 节点 tooltip（hover 触发）。
      {
        type: 'tooltip',
        key: 'node-tooltip',
        trigger: 'hover',
        getContent: (evt: IElementEvent) => {
          const id = (evt as unknown as { target?: { id?: string } }).target?.id as string
          if (!id) return ''
          const n = props.nodes.find((x) => x.id === id)
          if (!n) return ''
          const nb = neighborCount.value[id] ?? 0
          const kind = n.kind ?? 'UNKNOWN'
          const size = n.size ?? 0
          const cyc = props.cycleOf?.[id]
          const cycleRow = cyc?.length
            ? `<div class="t-row"><span>环</span><b class="t-cycle">环 #${cyc.join(', #')}</b></div>`
            : ''
          return `<div class="g6-tooltip">
            <div class="t-title">${n.label}</div>
            <div class="t-row"><span>kind</span><b>${kind}</b></div>
            <div class="t-row"><span>规模</span><b>${size}</b></div>
            <div class="t-row"><span>邻居</span><b>${nb}</b></div>
            ${cycleRow}
          </div>`
        },
      },
    ],
  }
}

// 把业务数据映射为 G6 NodeData/EdgeData（业务字段放 data 供样式回调使用）。
function toG6Data(): { nodes: NodeData[]; edges: EdgeData[] } {
  const nodes: NodeData[] = props.nodes.map((n) => ({
    id: n.id,
    data: { ...n },
  }))
  const edges: EdgeData[] = props.edges.map((e) => ({
    id: `${e.source}->${e.target}->${e.type}`,
    source: e.source,
    target: e.target,
    data: { ...e },
  }))
  return { nodes, edges }
}

function render(): void {
  if (!graph) return
  graph.setData(toG6Data())
  void graph.render()
}

function fitView(): void {
  void graph?.fitView()
}

function zoom(delta: number): void {
  if (!graph) return
  const center = graph.getViewportCenter()
  void graph.zoomBy(delta, undefined, center)
}

// 聚焦环成员（若在当前视图内）。
const visibleCycleIds = computed<string[]>(() => {
  const keys = props.cycleOf ? Object.keys(props.cycleOf) : []
  if (!keys.length) return []
  const ids = new Set(props.nodes.map((n) => n.id))
  return keys.filter((id) => ids.has(id))
})

function focusCycles(): void {
  const ids = visibleCycleIds.value
  if (!ids.length) return
  void graph?.focusElement(ids)
}

onMounted(() => {
  if (!container.value) return
  graph = new Graph(buildOptions())
  graph.on(NodeEvent.CLICK, (evt) => {
    const id = (evt as { target?: { id?: string } }).target?.id
    // 选点模式（路径起点/终点）开启时，单击进入选点；否则走下钻。
    if (id && selectMode.value) emit('node-select', id)
    else if (id) emit('node-click', id)
  })
  // 画布空白点击：清空选择（可选）。
  graph.on(GraphEvent.AFTER_RENDER, () => {})
  render()
})

// ---- D3 选点模式：路径起点/终点 ----
const selectMode = ref(false)

function toggleSelectMode(): void {
  selectMode.value = !selectMode.value
}

defineExpose({ setSelectMode: (v: boolean) => { selectMode.value = v } })

onBeforeUnmount(() => {
  graph?.destroy()
  graph = null
})

// 数据变化时重渲染（保持图实例，更新数据）。
watch(() => [props.nodes, props.edges], render, { deep: true })
</script>

<template>
  <div class="canvas-wrap">
    <div ref="container" class="graph-canvas" />
    <div class="toolbar">
      <button title="适应视图" @click="fitView">⛶</button>
      <button title="放大" @click="zoom(1.2)">＋</button>
      <button title="缩小" @click="zoom(0.8)">－</button>
      <button
        v-if="visibleCycleIds.length"
        title="聚焦环成员"
        class="cycle-btn"
        @click="focusCycles"
      >◉</button>
      <button
        title="路径选点（起点/终点）"
        class="route-btn"
        :class="{ active: selectMode }"
        @click="toggleSelectMode"
      >◎</button>
    </div>
    <div v-if="selectMode" class="select-hint">
      路径选点模式：点击节点作为起点（再点另一节点为终点）
    </div>
    <div v-else-if="visibleCycleIds.length" class="cycle-hint">
      <span class="dot" /> {{ visibleCycleIds.length }} 个环成员（红框），点击节点查看环归属
    </div>
  </div>
</template>

<style scoped>
.canvas-wrap {
  position: relative;
  width: 100%;
  height: 100%;
}
.graph-canvas {
  width: 100%;
  height: 100%;
  background: #fff;
}
.toolbar {
  position: absolute;
  top: 12px;
  right: 12px;
  display: flex;
  gap: 6px;
  padding: 4px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.06);
}
.toolbar button {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  color: #4e5969;
}
.toolbar button:hover {
  background: #f2f3f5;
  color: #165dff;
}
.toolbar button.cycle-btn {
  color: #e8684a;
}
.toolbar button.route-btn {
  color: #165dff;
}
.toolbar button.route-btn.active {
  background: #e8f3ff;
}
.select-hint {
  position: absolute;
  bottom: 12px;
  left: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 12px;
  color: #165dff;
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid #c6e2ff;
  border-radius: 12px;
}
.cycle-hint {
  position: absolute;
  bottom: 12px;
  left: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 12px;
  color: #e8684a;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid #fde2e2;
  border-radius: 12px;
}
.cycle-hint .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #e8684a;
}

:global(.g6-tooltip) {
  padding: 8px 10px;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.12);
  font-size: 12px;
  min-width: 120px;
}
:global(.g6-tooltip .t-title) {
  font-weight: 600;
  color: #1f2329;
  margin-bottom: 4px;
}
:global(.g6-tooltip .t-row) {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: #86909c;
  line-height: 1.8;
}
:global(.g6-tooltip .t-row b) {
  color: #1f2329;
  font-weight: 500;
}
:global(.g6-tooltip .t-cycle) {
  color: #e8684a;
}
</style>
