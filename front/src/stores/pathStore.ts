// 路径分析 store（D3-2）：A→B 具体依赖路径。
// 通过逐层 /fanout?depth=1 的邻接子图在前端 BFS 求最短路径，高亮路径节点/边。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { GraphEdge, GraphNode } from '../types'

export interface PathNode {
  qn: string
  label: string
}

export const usePathStore = defineStore('path', () => {
  /** 起点/终点（选点模式选中）。 */
  const from = ref<PathNode | null>(null)
  const to = ref<PathNode | null>(null)
  /** 已求出的路径节点序列（含起点/终点）。 */
  const path = ref<GraphNode[]>([])
  /** 路径边（source->target）。 */
  const pathEdges = ref<GraphEdge[]>([])
  const searching = ref(false)
  const error = ref('')

  /** 当前是否正在选点（起点或终点未齐）。 */
  const selecting = computed<boolean>(() => !from.value || !to.value)

  /** 提示文案。 */
  const hint = computed<string>(() => {
    if (!from.value) return '点击 ◎ 进入选点，先点起点'
    if (!to.value) return `起点 ${from.value.label}，再点终点`
    return `${from.value.label} → ${to.value.label}：${path.value.length ? path.value.length + ' 跳' : '未求路径'}`
  })

  function clear(): void {
    from.value = null
    to.value = null
    path.value = []
    pathEdges.value = []
    error.value = ''
  }

  function setStart(n: GraphNode): void {
    from.value = { qn: n.id, label: n.label }
    to.value = null
    path.value = []
    pathEdges.value = []
    error.value = ''
  }

  function setEnd(n: GraphNode): void {
    to.value = { qn: n.id, label: n.label }
    path.value = []
    pathEdges.value = []
    error.value = ''
  }

  /** 记录 BFS 求出的路径（节点 + 边，边 key = source->target）。 */
  function setPath(nodes: GraphNode[], edges: GraphEdge[]): void {
    path.value = nodes
    pathEdges.value = edges
  }

  /** 路径高亮集合（供 GraphCanvas 渲染）。 */
  const highlight = computed<{ nodes: Set<string>; edges: Set<string> }>(() => {
    const nodes = new Set<string>()
    const edges = new Set<string>()
    for (const n of path.value) nodes.add(n.id)
    for (const e of pathEdges.value) edges.add(`${e.source}->${e.target}`)
    return { nodes, edges }
  })

  return { from, to, path, pathEdges, searching, error, selecting, hint, highlight, clear, setStart, setEnd, setPath }
})
