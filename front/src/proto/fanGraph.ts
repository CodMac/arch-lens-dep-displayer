// L2 扇入扇出图（蝴蝶）：数据结构 + 布局。
// 图数据（BFS / 统计 / 节点明细）全部由后端 /fan 预计算，前端只做布局与渲染。

export type FanGranularity = 'package' | 'type' | 'method'
export type FanSide = 'center' | 'up' | 'down'

export interface FanNode {
  id: string
  label: string
  kind: string
  side: FanSide
  depth: number
  /** 跨包（类/方法粒度下用于强调越界）。 */
  crossPkg?: boolean
  /** 隐式生成（L3 徽标用，后端 elements.is_sugar）。 */
  isSugar?: boolean
  /** 源码语法结构（lambda / 匿名类 / 代码块，后端 elements.is_impl_detail）；L2 折叠开关用。 */
  isImplDetail?: boolean
  /** 宿主类 QN（方法粒度）。 */
  hostType?: string
  /** 节点类型（方法粒度用）。 */
  methodKind?: string
  /** 蝴蝶布局固定坐标（由视图计算）。 */
  x?: number
  y?: number
}

export interface FanEdge {
  source: string
  target: string
  type: string
  types?: string[]
  count?: number
  crossPkg?: boolean
}

export interface FanStats {
  /** 左翼（上游）去重类数 / 包数。 */
  upCount: number
  /** 右翼（下游）去重类数 / 包数。 */
  downCount: number
  /** 跨包越界边数。 */
  crossPkg: number
  /** 直接依赖数（depth=1）。 */
  directUp: number
  directDown: number
  /** 被分析对象的加权扇入/扇出（引用次数）。 */
  fanIn: number
  fanOut: number
}

/** 单个节点的扇入扇出明细（点击节点时展示）。 */
export interface FanNodeStat {
  id: string
  /** 加权扇入（引用次数）。 */
  fanIn: number
  /** 加权扇出。 */
  fanOut: number
  /** 直接上游（谁依赖我）：id + 次数 + 类型。 */
  inNeighbors: { id: string; count: number; types: string[] }[]
  /** 直接下游（我依赖谁）：id + 次数 + 类型。 */
  outNeighbors: { id: string; count: number; types: string[] }[]
}

export interface FanPayload {
  target: string
  granularity: FanGranularity
  nodes: FanNode[]
  edges: FanEdge[]
  stats: FanStats
  /** 数据里出现过的全部关系类型（供切片按钮）。 */
  relationTypes: string[]
  /** 节点 QN → 扇入扇出明细（基于完整图，不受深度/切片影响）。 */
  nodeStats: Record<string, FanNodeStat>
}

/** 蝴蝶布局：中心置中，上游左翼、下游右翼，按层数横向递进、同层纵向均分。 */
export function applyButterflyLayout(payload: FanPayload, opts?: { xGap?: number; yGap?: number }): void {
  const xGap = opts?.xGap ?? 240
  const yGap = opts?.yGap ?? 72
  const bySide = (side: FanSide): FanNode[] => payload.nodes.filter((n) => n.side === side)
  for (const n of payload.nodes) if (n.side === 'center') { n.x = 0; n.y = 0 }
  const place = (list: FanNode[], dir: -1 | 1): void => {
    const layers = new Map<number, FanNode[]>()
    for (const n of list) {
      const arr = layers.get(n.depth)
      if (arr) arr.push(n)
      else layers.set(n.depth, [n])
    }
    for (const [d, arr] of layers) {
      arr.sort((a, b) => a.id.localeCompare(b.id))
      const total = (arr.length - 1) * yGap
      arr.forEach((n, i) => {
        n.x = dir * d * xGap
        n.y = i * yGap - total / 2
      })
    }
  }
  place(bySide('up'), -1)
  place(bySide('down'), 1)
}
