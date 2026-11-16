<script setup lang="ts">
// 原型专用 G6 v5 画布封装。
// 特性：
//  - 节点形状编码：CLASS=圆角矩形、INTERFACE=虚线框矩形、ENUM/ANNOTATION=胶囊、PACKAGE(锚点)=灰显圆角矩形
//  - 环描边：包间环红（点+环边组成可见红环）/ 包内环紫；高扇入橙 / 高扇出绿；可展开包蓝
//  - 边宽统一 1.5（×N 数字标签表达依赖条数）；CONTAIN 虚线背景
//  - 单击节点下钻（emit node-click）；双击弹成员面板（emit node-dblclick）
//  - 点击边 → emit edge-click（父组件弹 Drill-down）
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { Graph, NodeEvent, EdgeEvent, ComboEvent } from '@antv/g6'
import type { GraphOptions, NodeData, EdgeData, ComboData as GComboData, IElementEvent, Element, NodeBadgeStyleProps } from '@antv/g6'
import { registerQnCombo } from './QnCombo'

registerQnCombo()

interface ProtoNode {
  id: string
  label: string
  kind: string
  size?: number
  anchor?: boolean
  cycle?: boolean
  highFanin?: boolean
  highFanout?: boolean
  expandable?: boolean
  depth?: number
  composite?: unknown[]
  parent?: string
  /** 11.19 L2 蝴蝶：center/up/down，用于左右翼配色。 */
  side?: 'center' | 'up' | 'down'
  /** 11.19 L2 蝴蝶：固定坐标（fixedLayout 时使用，不跑自动布局）。 */
  x?: number
  y?: number
}

interface ProtoEdge {
  source: string
  target: string
  type: string
  types?: string[]
  count?: number
  crossPkg?: boolean
}

interface ProtoCombo {
  id: string
  label: string
  parent?: string
  size?: number
  depth?: number
  collapsed?: boolean
  /** 子包数（L1 角标提示）。 */
  childCount?: number
  /** 环类型：1=跨包环（红描边）/ 2=包内环（紫描边）。 */
  cycleKind?: number
  /** 高扇入（橙描边）。 */
  highFanin?: boolean
  /** 违规向上汇聚计数（决策 8 / 11.17）：cycle=子树内环组数 / fanin=子树内高扇入类数 / fanout=子树内高扇出类数。 */
  violation?: { cycle: number; fanin?: number; fanout: number }
}

const props = defineProps<{
  nodes: ProtoNode[]
  edges: ProtoEdge[]
  combos?: ProtoCombo[]
  kindColors: Record<string, string>
  cycleOf?: Record<string, number[]>
  /** 环类型：1=跨包环（红）/ 2=包内环（紫）。 */
  cycleKindOf?: Record<string, number>
  /** 包间环边标记：source->target 属于某包间环（两端同环组），红实线加粗，组成可见红环。 */
  cycleEdgeKeys?: Set<string>
  highFaninNodes?: Set<string>
  highFanoutNodes?: Set<string>
  /** 主框包 id 集合（11.6 多主框）：所有下钻展开的主框 combo，高亮标识 + 放大展示。 */
  mainComboIds?: string[]
  /** 可折叠包集合（11.5 #57）：下钻链上的祖先包 + 主框，右键菜单显示「折叠」回退一层。 */
  collapsibleIds?: Set<string>
  /** 11.13 缺陷概览：高亮的环链路节点集合（点击环链路时高亮整条环）。 */
  highlightNodeIds?: Set<string>
  /** 11.13 缺陷概览：高亮的环链路边集合（`source->target`）。 */
  highlightEdgeKeys?: Set<string>
  /** 11.15：节点 hover tooltip 信息（与节点详情弹窗基本信息一致）。 */
  nodeTipInfo?: Record<string, { label: string; kind: string; rows: { k: string; v: string }[]; tags: string[] }>
  /** 11.19 L2 蝴蝶：使用节点自带 x/y 固定坐标，不跑自动布局。 */
  fixedLayout?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-click', id: string): void
  (e: 'node-dblclick', id: string): void
  (e: 'combo-click', id: string): void
  (e: 'combo-dblclick', id: string): void
  (e: 'combo-collapse', id: string): void
  (e: 'node-action', id: string, action: 'drill' | 'collapse' | 'source'): void
  (e: 'edge-click', source: string, target: string): void
}>()

const container = ref<HTMLDivElement>()
let graph: Graph | null = null

const hasCombo = () => !!props.combos?.length

// 双向边检测：存在反向边（B→A）时，该边画二次曲线错开，避免重叠。
const reverseSet = new Set<string>()
function isBidirectional(d: EdgeData): boolean {
  const s = d.source as string
  const t = d.target as string
  if (reverseSet.has(`${s}->${t}`)) return true
  if (props.edges.some((e) => e.source === t && e.target === s)) {
    reverseSet.add(`${s}->${t}`)
    return true
  }
  return false
}

function nodeType(d: NodeData): string {
  const k = (d.data?.kind as string) ?? 'UNKNOWN'
  if (k === 'CLASS') return 'rect'
  if (k === 'INTERFACE') return 'rect' // 虚线框由 lineDash 表达
  if (k === 'ENUM' || k === 'ANNOTATION') return 'ellipse'
  if (k === 'PACKAGE') return 'rect' // 锚点
  return 'circle'
}

function buildOptions(): GraphOptions {
  const layout = props.fixedLayout
    ? undefined // 11.19：节点自带 x/y（蝴蝶布局），不跑自动布局
    : hasCombo()
    ? {
        type: 'combo-combined',
        // 11.5（#56 重叠修复）：combo 内子元素碰撞检测用真实尺寸（node.size），不再硬编码 nodeSize=40。
        // combo-combined 的 layout 回调对每个 combo 层级递归执行，createTemporaryGraphData 会把子元素
        // size 设为 getNodeLikeSize 的真实框尺寸；原固定 40px 圆 vs 实际 200px+ 子包框导致主框内子包互相重叠。
        // nodeSize: 'node.size' + nodeSpacing 12 让碰撞检测按真实框散开；linkDistance 适当加大避免边引力拉重叠。
        layout: () => ({
          type: 'force',
          preventOverlap: true,
          nodeSize: 'node.size',
          nodeSpacing: 12,
          linkDistance: 140,
        }),
        comboSpacing: 24,
        comboPadding: 20,
      }
    : props.nodes.length > 60
      ? { type: 'force', preventOverlap: true, linkDistance: 80 }
      : { type: 'dagre', rankdir: 'LR', nodesep: 24, ranksep: 60 }

  const maxSize = Math.max(1, ...props.nodes.map((n) => n.size ?? 0))
  // 11.1 问题1：节点框大小封顶。原公式 30+round(size/maxSize*40) 在 size=10 时 sizeOf=70，
  // 渲染框达 448px，是兄弟包 combo 框（88-102px）的 4-5 倍。改为窄区间线性映射：
  // sizeOf ∈ [16,22]，渲染框约 90-140px，与包框协调，同时保留 size 相对大小可辨性。
  // 注意：锚点节点（kind=PACKAGE）保持小尺寸（16），不参与主框内类节点标定。
  const sizeOf = (size: number, kind: string): number => {
    if (kind === 'PACKAGE') return 16 // 锚点小方块
    return 16 + Math.round((size / maxSize) * 6)
  }

  const isCycle = (d: NodeData): boolean => !!props.cycleOf?.[d.id as string]
  // 环描边色：跨包环=红 / 包内环=紫（5.57 区分）；未标记时按红（默认跨包）。
  const cycleKind = (d: NodeData): number => (props.cycleKindOf?.[d.id as string] as number) ?? 1
  const isHighFanin = (d: NodeData): boolean => !!props.highFaninNodes?.has(d.id as string)
  const isHighFanout = (d: NodeData): boolean => !!props.highFanoutNodes?.has(d.id as string)
  /** 包间环边：两端同属某包间环（红实线加粗，组成可见红环）。 */
  const isCycleEdge = (d: EdgeData): boolean => {
    if (!props.cycleEdgeKeys?.size) return false
    return props.cycleEdgeKeys.has(`${d.source}->${d.target}`)
  }
  /** 11.13：环链路高亮（点击缺陷概览的环链路后，整条环节点/边加粗高亮）。 */
  const isHighlightNode = (d: { id?: string }): boolean => !!props.highlightNodeIds?.has(String(d.id))
  const isHighlightEdge = (d: EdgeData): boolean =>
    !!props.highlightEdgeKeys?.has(`${d.source}->${d.target}`)
  const isAnchor = (d: NodeData): boolean => !!((d.data as ProtoNode | undefined)?.anchor)
  const isExpandable = (d: NodeData): boolean => ((d.data as ProtoNode | undefined)?.kind) === 'PACKAGE' && !!((d.data as ProtoNode | undefined)?.expandable)
  // 节点 kind 徽标文本：PACKAGE=包 / CLASS=类 / INTERFACE=接口 / ENUM=枚举 / ANNOTATION=注解 / UNKNOWN=未知。
  const KIND_BADGE: Record<string, string> = {
    PACKAGE: '包', CLASS: '类', INTERFACE: '接口', ENUM: '枚举', ANNOTATION: '注解', UNKNOWN: '未知',
  }

  return {
    container: container.value as HTMLElement,
    autoResize: true,
    autoFit: 'view',
    // 全局关闭动画：G6 v5 增量更新时元素销毁与 enter 动画 after 回调竞态（elementMap[id] 已空）
    // 会导致 `Cannot read properties of undefined (reading 'onUpdate')` 崩溃。node↔combo 转换
    // 需要 clear+重建，无动画时更新路径同步执行，规避该竞态。
    animation: false,
    layout,
    node: {
      type: (d: NodeData) => nodeType(d),
      style: {
        size: (d: NodeData) => sizeOf((d.data?.size as number) ?? 0, String((d.data?.kind as string) ?? '')),
        // 中性底色（6.2 去掉 kind 色块）：fill 不再按 kind 上色，避免与环/高扇入描边抢视觉。
        fill: (d: NodeData) => {
          // 11.19 L2 蝴蝶：左右翼淡色底，中心白底。
          const side = d.data?.side as string | undefined
          if (side === 'center') return '#fff1f0'
          if (side === 'up') return '#eef4ff'
          if (side === 'down') return '#eaf7f0'
          if (isAnchor(d)) return '#f2f3f5'
          return '#ffffff'
        },
        stroke: (d: NodeData) => {
          if (isHighlightNode(d)) return '#e8684a' // 11.13 环链路高亮：红
          // 11.19 L2 蝴蝶：中心红、上游蓝、下游绿（左右翼一眼可分）。
          const side = d.data?.side as string | undefined
          if (side === 'center') return '#e8684a'
          if (side === 'up') return '#165dff'
          if (side === 'down') return '#2f9e6e'
          if (isCycle(d)) return cycleKind(d) === 2 ? '#9270CA' : '#e8684a' // 包内环紫 / 跨包环红
          if (isHighFanin(d)) return '#F77234' // 高扇入橙
          if (isHighFanout(d)) return '#2f9e6e' // 高扇出绿（6.2 新增，区分扇入橙）
          if (isAnchor(d)) return '#c9cdd4'
          if (isExpandable(d)) return '#5B8FF9'
          return '#c0c4cc'
        },
        lineWidth: (d: NodeData) => {
          if (isHighlightNode(d)) return 4
          const side = d.data?.side as string | undefined
          if (side === 'center') return 3.5
          if (side) return 2
          if (isCycle(d) || isHighFanin(d) || isHighFanout(d)) return 3
          if (isAnchor(d) || isExpandable(d)) return 2
          return 1.5
        },
        lineDash: (d: NodeData) => {
          const k = (d.data?.kind as string) ?? ''
          if (isAnchor(d)) return [3, 3]
          if (k === 'INTERFACE') return [4, 2]
          return []
        },
        radius: 8,
        labelText: (d: NodeData) => (d.data?.label as string) ?? String(d.id),
        labelPlacement: 'bottom',
        labelFontSize: 11,
        labelFill: (d: NodeData) => (isAnchor(d) ? '#86909c' : ((d.data?.depth as number) ?? 0) > 0 ? '#6b7280' : '#333'),
        // 类型文本（6.2）：节点右上角 kind 徽标，直观表达节点类型。
        // 10.5：恢复包节点（PACKAGE）kind 徽标——用户反馈包节点也要显示 kind 文字（10.3 移除被否定）。
        badge: true,
        badgePalette: ['#ffffff'],
        badges: (d: NodeData): NodeBadgeStyleProps[] => {
          const k = (d.data?.kind as string) ?? 'UNKNOWN'
          const badges: NodeBadgeStyleProps[] = []
          const t = KIND_BADGE[k] ?? '未知'
          // 11.1 问题4：kind 徽标大小随节点框尺寸缩放。锚点小方块（PACKAGE size=16）不显示
          // 徽标（避免 9px 配 16px 突兀）；类/接口/枚举节点按 sizeOf 标定（16-22）推导 fontSize。
          const n = sizeOf((d.data?.size as number) ?? 0, k)
          const isAnchorNode = isAnchor(d)
          if (isAnchorNode) return badges // 锚点：无徽标，保持纯小方块
          const fs = Math.max(7, Math.min(10, Math.round(5 + n * 0.22)))
          const pad = Math.max(1, Math.round(fs * 0.45))
          badges.push({
            text: t,
            placement: 'right-top',
            offsetX: 2,
            offsetY: 2,
            padding: [1, pad, 1, pad],
            fontSize: fs,
            backgroundFill: '#eef1f4',
            backgroundStroke: '#d5dae0',
            backgroundLineWidth: 1,
            backgroundRadius: fs + 1,
            fill: '#4e5969',
          })
          // 高扇入/高扇出（11.1 问题4）：与 kind 徽标同列（right-top 下排），文字化 + 统一胶囊样式，
          // 消除原 ⇐/⇒ 符号（left-top）与 combo 违规角标文字（right-top）的左右/符号/文字变形。
          // 单节点无计数数据（仅布尔标记），用文字而非 ▲N 计数。
          // 11.17：高扇入与高扇出可同时存在（原 else if 会吞掉高扇出），两者都推入，
          // 由 G6 按同 placement 自动纵向堆叠。
          if (isHighFanin(d)) badges.push({ text: '高扇入', placement: 'right-top', offsetX: 2, offsetY: 2, padding: [1, Math.max(1, Math.round(fs * 0.35)), 1, Math.max(1, Math.round(fs * 0.35))], fontSize: fs, backgroundFill: '#fff0e6', backgroundStroke: '#F77234', backgroundLineWidth: 1, backgroundRadius: fs + 1, fill: '#F77234' })
          if (isHighFanout(d)) badges.push({ text: '高扇出', placement: 'right-top', offsetX: 2, offsetY: 2, padding: [1, Math.max(1, Math.round(fs * 0.35)), 1, Math.max(1, Math.round(fs * 0.35))], fontSize: fs, backgroundFill: '#e6f7ee', backgroundStroke: '#2f9e6e', backgroundLineWidth: 1, backgroundRadius: fs + 1, fill: '#2f9e6e' })
          return badges
        },
      },
    },
    combo: {
      type: 'qn-combo',
      style: {
        size: (d: GComboData) => {
          // 11.5 #57：按离主框的层级距离（nestLevel）递进缩小。主框(nestLevel=0)最大，
          // 父框(1)/祖父框(2)…逐级缩小；周边包已改 node 形态统一变小。此为布局前占位尺寸，
          // 实际渲染尺寸由 combo-combined 布局按子元素自动撑大（外层必然大于内层）。
          const s = (d.data?.size as number) ?? 0
          const lvl = (d.data?.nestLevel as number) ?? 0
          const scale = Math.max(0.45, 1 - lvl * 0.22)
          return [Math.round((80 + Math.min(s, 200)) * scale), Math.round((60 + Math.min(s, 120)) * scale)]
        },
        radius: 10,
        // 11.13：移除模块色块后，combo 回归中性 depth 分档（同色系浅蓝，不抢违规描边视觉）。
        fill: (d: GComboData) => {
          if (props.mainComboIds?.includes(d.data?.id as string)) return '#e8f1ff' // 主框淡蓝底
          const dep = (d.data?.depth as number) ?? 0
          return dep === 0 ? '#eaf3ff' : dep === 1 ? '#f2f7ff' : '#f7fafd'
        },
        stroke: (d: GComboData) => {
          // 11.13 环链路高亮优先（点击缺陷概览的环链路）。
          if (isHighlightNode(d)) return '#e8684a'
          // 主框优先：高亮描边（最外层视觉锚点）。
          if (props.mainComboIds?.includes(d.data?.id as string)) return '#165dff'
          const ck = (d.data?.cycleKind as number | undefined)
          if (ck === 2) return '#9270CA' // 包内环紫
          if (ck === 1) return '#e8684a' // 跨包环红
          if ((d.data?.highFanin as boolean | undefined)) return '#F77234' // 高扇入橙
          if ((d.data?.highFanout as boolean | undefined)) return '#2f9e6e' // 高扇出绿（11.7：包成 combo 后仍标识）
          const dep = (d.data?.depth as number) ?? 0
          return dep === 0 ? '#5B8FF9' : dep === 1 ? '#91caff' : '#c3d8f2'
        },
        lineWidth: (d: GComboData) => {
          if (isHighlightNode(d)) return 4 // 11.13 环链路高亮加粗
          if (props.mainComboIds?.includes(d.data?.id as string)) return 4 // 主框加粗
          const ck = (d.data?.cycleKind as number | undefined)
          if (ck || (d.data?.highFanin as boolean | undefined) || (d.data?.highFanout as boolean | undefined)) return 3
          return (d.data?.depth as number) === 0 ? 2 : 1.2
        },
        padding: 16,
        // 两行式（决策 6 方案 B）：第一行短名（label，醒目）+ 灰色完整 QN（qn-label 副形状）。
        labelText: (d: GComboData) => (d.data?.label as string) ?? String(d.id).split('.').pop() ?? String(d.id),
        labelPlacement: 'top',
        labelFontSize: 11,
        labelFontWeight: 600,
        labelTextAlign: 'center',
        labelLineHeight: 14,
        labelWordWrap: true,
        labelWordWrapWidth: 160,
        labelMaxLines: 2,
        labelTextOverflow: '',
        labelFill: (d: GComboData) => {
          return ((d.data?.depth as number) ?? 0) === 0 ? '#1f2329' : '#4e5969'
        },
        // 折叠壳：普通气泡缩小保留（无虚线、无 marker），子节点已由父组件数据驱动移除。
        collapsedMarker: false,
      },
    },
    edge: {
      type: (d: EdgeData) => (isBidirectional(d) ? 'quadratic' : 'line'),
      style: {
        stroke: (d: EdgeData) => {
          // 11.13 环链路高亮：整条环的边加粗红色。
          if (isHighlightEdge(d)) return '#e8684a'
          // 包间环边（6.2 决策 7）：两端同属包间环 → 红实线，点+边组成可见红环。
          if (isCycleEdge(d)) return '#e8684a'
          if (d.data?.crossPkg as boolean) return '#e8684a' // 跨包边：红
          return d.data?.type === 'CONTAIN' ? '#e5e6eb' : '#bfc6d3'
        },
        lineWidth: (d: EdgeData) => {
          if (d.data?.type === 'CONTAIN') return 1
          if (isHighlightEdge(d)) return 3.5 // 11.13 环链路高亮
          if (isCycleEdge(d)) return 2.5 // 环边加粗，突出环
          return 1.5 // 6.1 统一线宽：不再按依赖条数（count）变粗细，粗细留给 ×N 数字标签表达
        },
        lineDash: (d: EdgeData) => {
          if (d.data?.crossPkg as boolean) return [6, 3]
          return d.data?.type === 'CONTAIN' ? [4, 4] : []
        },
        endArrow: true,
        // 双向边：二次曲线错开（两方向各偏一侧），避免重叠
        curveOffset: (d: EdgeData) => (isBidirectional(d) ? 20 : 0),
        strokeOpacity: (d: EdgeData) => (d.data?.type === 'CONTAIN' ? 0.5 : 0.9),
        // 边权重数字（子边数量）：边中点显示 count
        labelText: (d: EdgeData) => {
          const count = (d.data?.count as number) ?? 0
          if (count > 1) return `×${count}`
          return ''
        },
        labelFontSize: 10,
        labelFill: '#165dff',
        labelBackground: true,
        labelBackgroundFill: '#fff',
        labelBackgroundOpacity: 0.85,
        labelBackgroundRadius: 6,
        labelPadding: [2, 4],
        labelPosition: 'center',
        labelOffset: (d: EdgeData) => (isBidirectional(d) ? [0, -10] : [0, 0]),
      },
    },
    behaviors: [
      'zoom-canvas',
      'drag-canvas',
      'drag-element',
      'hover-activate',
      // 折叠由父组件数据驱动（collapsed 字段），不在画布内用 collapse-expand，避免与聚焦下钻点击冲突。
    ],
    plugins: [
      {
        type: 'contextmenu',
        key: 'node-menu',
        trigger: 'contextmenu',
        getItems: (evt: IElementEvent) => {
          const id = (evt as unknown as { target?: { id?: string } }).target?.id as string
          if (!id) return []
          const items = [{ name: '下钻', value: 'drill' }]
          // 折叠（交互收敛）：任意可见非叶子包可折叠（丢弃直接子包及其子孙，自身缩小保留）。
          if (props.collapsibleIds?.has(id)) items.push({ name: '折叠', value: 'collapse' })
          items.push({ name: '查看源码', value: 'source' })
          return items
        },
        onClick: (value: string, _target: HTMLElement, current: Element) => {
          const id = current.id as string
          if (!id) return
          emit('node-action', id, value as 'drill' | 'collapse' | 'source')
        },
      },
      {
        type: 'tooltip',
        key: 'node-tooltip',
        trigger: 'hover',
        getContent: (evt: IElementEvent) => {
          const id = (evt as unknown as { target?: { id?: string } }).target?.id as string
          if (!id) return ''
          const n = props.nodes.find((x) => x.id === id)
          if (!n) return ''
          // 11.15：优先用父组件提供的详情口径（与节点详情弹窗基本信息一致）。
          const info = props.nodeTipInfo?.[id]
          if (info) {
            const rows = info.rows
              .map((r) => `<div class="t-row"><span>${r.k}</span><b>${r.v}</b></div>`)
              .join('')
            const tags = info.tags.length
              ? `<div class="t-tags">${info.tags.map((t) => `<span class="t-tag">${t}</span>`).join('')}</div>`
              : '<div class="t-tags"><span class="t-tag t-none">无违规</span></div>'
            return `<div class="g6-tooltip">
              <div class="t-title">${info.label}</div>
              <div class="t-row"><span>类型</span><b>${info.kind}</b></div>
              ${rows}
              ${tags}
            </div>`
          }
          const cyc = props.cycleOf?.[id]
          const cycleRow = cyc?.length ? `<div class="t-row"><span>环</span><b class="t-cycle">环 #${cyc.join(', #')}</b></div>` : ''
          return `<div class="g6-tooltip">
            <div class="t-title">${n.label}</div>
            <div class="t-row"><span>kind</span><b>${n.kind}</b></div>
            <div class="t-row"><span>规模</span><b>${n.size ?? 0}</b></div>
            ${n.anchor ? '<div class="t-row"><span>锚点</span><b>外部包</b></div>' : ''}
            ${cycleRow}
          </div>`
        },
      },
    ],
  }
}

function toG6Data(): { nodes: NodeData[]; edges: EdgeData[]; combos: GComboData[] } {
  const nodes: NodeData[] = props.nodes.map((n) => ({
    id: n.id,
    combo: n.parent || undefined,
    data: { ...n },
    // 11.19 蝴蝶固定布局：把坐标写进 style，G6 不再跑自动布局。
    ...(props.fixedLayout && n.x !== undefined && n.y !== undefined
      ? { style: { x: n.x, y: n.y } }
      : {}),
  }))
  // 防御：过滤掉两端节点不在图内的边（上游过滤时序可能短暂不一致，G6 会因找不到节点而抛错）。
  // 注意端点可能是 combo（L1 汇聚边两端是模块包 combo），故 ids 需含 nodes + combos。
  const ids = new Set([...nodes.map((n) => n.id), ...(props.combos ?? []).map((c) => c.id)])
  const edges: EdgeData[] = props.edges
    .filter((e) => ids.has(e.source) && ids.has(e.target))
    .map((e) => ({
      id: `${e.source}->${e.target}->${e.type}`,
      source: e.source,
      target: e.target,
      data: { ...e },
    }))
  const combos: GComboData[] = (props.combos ?? []).map((c) => ({
    id: c.id,
    combo: c.parent || undefined,
    // qn 供自定义 combo 的 qn-label 副形状取完整 QN；mainCombo 标记主框（画折叠按钮）。
    data: { ...c, qn: c.id, mainCombo: props.mainComboIds?.includes(c.id) ?? false },
  }))
  return { nodes, edges, combos }
}

// 记录最近一次渲染的 combo id 集合，用于检测 node↔combo 身份转换（L1/L2 合并：叶子包单点 ↔ 容器 combo）。
// G6 v5 增量 diff 无法处理「同 id 在 node 与 combo 间切换」（如叶子包从单点变容器 combo），
// 会抛 'Node already exists' 且中止更新（旧数据残留）。检测到变化时先 clear 再加载新数据，
// 保持同一 graph 实例（避免重建导致 dev 钩子/事件失效），且全量移除无身份冲突。
let lastComboIds = ''
/** 渲染串行队列：多次 watch 并发触发 render 时按序执行，避免 clear/setData/render 交错。 */
let renderChain: Promise<void> = Promise.resolve()
function render(): void {
  if (!graph) return
  reverseSet.clear()
  const d = toG6Data()
  const comboIds = (d.combos ?? []).map((c) => c.id).sort().join('|')
  const needReset = lastComboIds !== '' && comboIds !== lastComboIds
  lastComboIds = comboIds
  renderChain = renderChain
    .catch(() => undefined)
    .then(async () => {
      if (!graph) return
      if (needReset) {
        // 全量清空（async，仅清 model+element，不触发布局，无空数据布局崩溃）。
        await graph.clear()
      }
      graph.setData(d)
      // 注：G6 v5 setData 增量 diff 在「边被切片/深度移除但端点仍存」时可能抛 Edge not found
      // 瞬时告警（不阻塞渲染，图仍正确）。已由 toG6Data 的节点存在性过滤兜底，此处不重试。
      await graph.render()
    })
  void renderChain.catch((e) => {
    // 渲染失败不应静默：暴露到控制台便于定位（dev 期）。
    console.error('[proto-canvas] render failed', e)
  })
}
/** 等待最近一次渲染完成（供 focusElement 在重建后再执行聚焦）。 */
function whenRendered(): Promise<void> {
  return renderChain.catch(() => undefined)
}

function currentLayoutMode(): 'combo' | 'force' | 'dagre' | 'fixed' {
  if (props.fixedLayout) return 'fixed'
  return hasCombo() ? 'combo' : props.nodes.length > 60 ? 'force' : 'dagre'
}
let builtMode: 'combo' | 'force' | 'dagre' | 'fixed' | '' = ''

function createGraph(): void {
  if (!container.value) return
  graph?.destroy()
  graph = new Graph(buildOptions())
  graph.on(NodeEvent.CLICK, (evt) => {
    const id = (evt as { target?: { id?: string } }).target?.id
    if (id) emit('node-click', id)
  })
  graph.on(NodeEvent.DBLCLICK, (evt) => {
    const id = (evt as { target?: { id?: string } }).target?.id
    if (id) emit('node-dblclick', id)
  })
  graph.on(EdgeEvent.CLICK, (evt) => {
    const e = (evt as { target?: { id?: string } }).target?.id as string | undefined
    if (!e) return
    // id 形如 source->target->type
    const [s, t] = e.split('->')
    if (s && t) emit('edge-click', s, t)
  })
  graph.on(ComboEvent.CLICK, (evt) => {
    // G6 v5 事件结构：evt.target = G6 元素（combo），evt.originalTarget = 命中的形状（子图形）。
    // 实测（probe_orig_target.js）：真实点击折叠按钮时 originalTarget 是 circle，
    // 其 .name 为空、.className === 'collapse-btn'（G6 upsert 把形状名作为 config.className）。
    const e = evt as { target?: { id?: string }; originalTarget?: { className?: string; name?: string } }
    const id = e.target?.id
    if (!id) return
    const ot = e.originalTarget
    if (ot && (ot.className === 'collapse-btn' || ot.name === 'collapse-btn')) {
      emit('combo-collapse', id)
      return
    }
    emit('combo-click', id)
  })
  graph.on(ComboEvent.DBLCLICK, (evt) => {
    // 11.4 双击下钻：单击=选中（combo-click），双击=下钻加载子节点。
    const e = evt as { target?: { id?: string }; originalTarget?: { className?: string } }
    const id = e.target?.id
    if (!id) return
    const ot = e.originalTarget
    if (ot && (ot.className === 'collapse-btn' || ot.className === 'corner-badge')) return // 徽标/折叠按钮不响应双击
    emit('combo-dblclick', id)
  })
  builtMode = currentLayoutMode()
  lastComboIds = (props.combos ?? []).map((c) => c.id).sort().join('|')
  render()
  // dev 验证钩子：暴露 graph 实例，供 puppeteer 以 graph.emit() 触发 canvas 事件。
  if (import.meta.env.DEV) (window as unknown as Record<string, unknown>).__protoGraph = graph
}

function fitView(): void {
  void graph?.fitView()
}

/** 聚焦主框包：放大到合适尺寸（主框占画布约 60% 宽/高）并居中，周边散列包缩留边缘。 */
function focusElement(id: string): void {
  void whenRendered().then(() => {
    if (!graph) return
    let bbox
    try {
      bbox = graph.getElementRenderBounds(id)
    } catch {
      bbox = null
    }
    if (!bbox || !bbox.min || !bbox.max) {
      void graph.focusElement(id)
      return
    }
  const bw = bbox.max[0] - bbox.min[0]
  const bh = bbox.max[1] - bbox.min[1]
  if (!bw || !bh) {
    void graph.focusElement(id)
    return
  }
    const [vw, vh] = graph.getSize()
    const target = 0.6 // 主框占画布 60% 宽/高（取较紧约束）
    const zoom = Math.min((vw * target) / bw, (vh * target) / bh)
    const cx = (bbox.min[0] + bbox.max[0]) / 2
    const cy = (bbox.min[1] + bbox.max[1]) / 2
    const vc = graph.getViewportCenter()
    // 以主框中心为锚缩放，锚点 viewport 位置不变。
    void graph.zoomTo(zoom, undefined, [cx, cy])
    // 主框中心经缩放后的 viewport 位置 = getViewportByCanvas(cx,cy)。
    const p = graph.getViewportByCanvas([cx, cy])
    // 平移到画布中心。
    void graph.translateBy([vc[0] - p[0], vc[1] - p[1]])
  })
}

onMounted(() => {
  createGraph()
})

onBeforeUnmount(() => {
  graph?.destroy()
  graph = null
})

watch(
  () => [props.nodes, props.edges],
  () => {
    if (builtMode !== currentLayoutMode()) createGraph()
    else render()
  },
  { deep: true },
)

// 11.13：环链路高亮变化 → 仅重绘（不重建布局）。
watch(
  () => [props.highlightNodeIds, props.highlightEdgeKeys],
  () => {
    if (graph) render()
  },
)

defineExpose({ fitView, focusElement })
</script>

<template>
  <div class="proto-canvas-wrap">
    <div ref="container" class="proto-graph-canvas" />
    <div class="proto-toolbar">
      <button title="适应视图" @click="fitView">⛶</button>
    </div>
  </div>
</template>

<style scoped>
.proto-canvas-wrap {
  position: relative;
  width: 100%;
  height: 100%;
}
.proto-graph-canvas {
  width: 100%;
  height: 100%;
  background: #fff;
}
.proto-toolbar {
  position: absolute;
  top: 12px;
  right: 12px;
  display: flex;
  gap: 6px;
  padding: 4px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid #e5e6eb;
  border-radius: 6px;
}
.proto-toolbar button {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  color: #4e5969;
}
.proto-toolbar button:hover {
  background: #f2f3f5;
  color: #165dff;
}
/* 11.15：节点 hover tooltip（G6 渲染到 body，需全局样式） */
:global(.g6-tooltip) {
  padding: 8px 10px;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.12);
  font-size: 12px;
  min-width: 140px;
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
:global(.g6-tooltip .t-tags) {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid #eef0f3;
}
:global(.g6-tooltip .t-tag) {
  padding: 0 5px;
  border-radius: 8px;
  font-size: 10px;
  background: #fdeceb;
  color: #e8684a;
}
:global(.g6-tooltip .t-tag.t-none) {
  background: #f2f3f5;
  color: #86909c;
}
</style>
