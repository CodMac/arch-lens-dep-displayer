// 图数据 store：当前层图数据 + 关系类型过滤 + D3 聚焦分析状态。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { GraphData, GraphEdge, GraphNode } from '../types'
import { CONTAIN_TYPE } from './dimension'

export const useGraphDataStore = defineStore('graphData', () => {
  const data = ref<GraphData>({ nodes: [], edges: [] })
  const loading = ref(false)
  const error = ref('')
  // 关系类型过滤：未勾选（false）的边类型被隐藏。
  const typeFilter = ref<Record<string, boolean>>({})

  // ---- D3 聚焦分析（扇入/扇出子图叠加，非下钻） ----
  const fanMode = ref<'' | 'in' | 'out'>('')
  const fanRoot = ref('')
  const fanDepth = ref(2)
  const fanLoading = ref(false)
  // 扇入/扇出子图（服务端 fan 接口返回的聚合边）。
  const fanData = ref<GraphData>({ nodes: [], edges: [] })

  const allTypes = computed<string[]>(() =>
    Array.from(new Set(data.value.edges.map((e) => e.type))),
  )

  /** 应用过滤后的边（CONTAIN 始终显示，作为层级骨架）。 */
  const visibleEdges = computed<GraphEdge[]>(() =>
    data.value.edges.filter((e) => {
      if (e.type === CONTAIN_TYPE) return true
      return typeFilter.value[e.type] !== false
    }),
  )

  function setData(d: GraphData): void {
    data.value = d
    error.value = ''
  }

  function toggleType(t: string, on: boolean): void {
    typeFilter.value = { ...typeFilter.value, [t]: on }
  }

  function setTypeFilter(f: Record<string, boolean>): void {
    typeFilter.value = { ...f }
  }

  /** 叠加扇入/扇出子图到当前视图（锚点合并，fan 根节点保留真实样式）。 */
  function mergeFan(fan: GraphData, root: string, mode: 'in' | 'out', depth: number): void {
    const base = data.value
    const seen = new Set(base.nodes.map((n) => n.id))
    const nodes = [...base.nodes]
    for (const n of fan.nodes) {
      if (seen.has(n.id)) continue
      nodes.push({ ...n, anchor: n.id !== root } as GraphNode & { anchor: boolean })
      seen.add(n.id)
    }
    // 边：当前层边 + fan 子图边（去重）。
    const edgeSeen = new Set(base.edges.map((e) => `${e.source}->${e.target}->${e.type}`))
    const edges = [...base.edges]
    for (const e of fan.edges) {
      const k = `${e.source}->${e.target}->${e.type}`
      if (edgeSeen.has(k)) continue
      edges.push(e)
      edgeSeen.add(k)
    }
    data.value = { nodes, edges }
    fanMode.value = mode
    fanRoot.value = root
    fanDepth.value = depth
  }

  /** 叠加路径子图到当前视图（D3-2）：新增节点全部标记为锚点（灰显），路径高亮由 PathStore 提供。 */
  function mergePath(nodes: GraphNode[], edges: GraphEdge[]): void {
    const seen = new Set(data.value.nodes.map((n) => n.id))
    const merged: GraphNode[] = [...data.value.nodes]
    for (const n of nodes) {
      if (seen.has(n.id)) continue
      merged.push({ ...n, anchor: true } as GraphNode & { anchor: boolean })
      seen.add(n.id)
    }
    const edgeSeen = new Set(data.value.edges.map((e) => `${e.source}->${e.target}->${e.type}`))
    const edges2 = [...data.value.edges]
    for (const e of edges) {
      const k = `${e.source}->${e.target}->${e.type}`
      if (edgeSeen.has(k)) continue
      edges2.push(e)
      edgeSeen.add(k)
    }
    data.value = { nodes: merged, edges: edges2 }
  }

  /** 清除扇入/扇出叠加（回到当前层视图）。 */
  function clearFan(): void {
    fanMode.value = ''
    fanRoot.value = ''
    fanData.value = { nodes: [], edges: [] }
  }

  function reset(): void {
    data.value = { nodes: [], edges: [] }
    typeFilter.value = {}
    error.value = ''
    clearFan()
  }

  return {
    data, loading, error, allTypes, typeFilter, visibleEdges,
    fanMode, fanRoot, fanDepth, fanLoading, fanData,
    setData, toggleType, setTypeFilter, mergeFan, mergePath, clearFan, reset,
  }
})
