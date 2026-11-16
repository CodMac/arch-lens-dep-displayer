// 前端数据模型：与后端 DTO 同构，字段与前端 model 一一对应。

// ---------- L1 全景 ----------

/** 违规摘要（仪表盘下可点击摘要行）。 */
export interface ViolationSummary {
  cycleGroups: number // 环组数
  cyclePkgs: number // 涉环包数
  highFaninPkgs: number // 高扇入包数
}

/** L1 全景负载。 */
export interface OverviewPayload {
  metrics: { pkgCount: number; typeCount: number; relCount: number; cycleGroups: number }
  violations: ViolationSummary
  nodes: {
    id: string
    label: string
    kind: string
    /** 子树包数（旧口径）。 */
    size: number
    /** 子树类规模（真实类数）。 */
    typeCount?: number
    children: number
    depth: number
    parent?: string
  }[]
  edges: { source: string; target: string; type: string; types: string[]; count: number }[]
  tree: { qn: string; parent: string; depth: number }[]
}

// ---------- L2 包内聚焦 ----------

/** L2 节点（本包类 + 外部包锚点）。 */
export interface L2Node {
  id: string
  label: string
  kind: 'CLASS' | 'INTERFACE' | 'ENUM' | 'ANNOTATION' | 'PACKAGE'
  /** 是否本包节点；false = 外部包锚点（灰显）。 */
  inPkg: boolean
  /** 成员数（类节点角标）。 */
  memberCount?: number
  /** 是否处于循环依赖链（红描边 + 呼吸灯）。 */
  cycle?: boolean
  size?: number
}

/** 边 Drill-down 明细（方法级调用）。 */
export interface EdgeCallDetail {
  src: string // 源方法 QN
  dst: string // 目标方法 QN
  type: string
  line: number
}

/** L2 边。 */
export interface L2Edge {
  source: string
  target: string
  type: string // 主类型
  types: string[]
  count: number
  calls: EdgeCallDetail[]
  crossPkg?: boolean // 跨包边
}

/** 类成员（双击类 → 成员面板）。 */
export interface MemberInfo {
  id: string
  label: string
  kind: string // METHOD / FIELD / CONSTRUCTOR
  signature: string
}

/** 类级依赖边（真实 relation 归属到宿主类）：包级边由它上卷而来，边详情可下钻到类。 */
export interface TypeEdge {
  source: string // 源类 QN
  target: string // 目标类 QN
  count: number
  types: string[]
}

/** 实际边（调用点级）：源方法/成员 → 目标方法/成员 + 调用点位置（文件/行号）。 */
export interface ActualEdge {
  sourceQn: string
  targetQn: string
  type: string
  file: string
  line: number
  count: number
}

/** 非叶子包的直接类（包下第一层类，与子包并列展示）。 */
export interface DirectType {
  id: string
  label: string
  kind: 'CLASS' | 'INTERFACE' | 'ENUM' | 'ANNOTATION'
  /** 类成员（方法/字段），用于类详情看板「成员」tab。 */
  members: MemberInfo[]
}

/** L2 包内聚焦负载。 */
export interface L2Payload {
  pkg: string
  nodes: L2Node[]
  edges: L2Edge[]
  members: Record<string, MemberInfo[]>
}

// ---------- L3 源码与元素树 ----------

export interface SourceElement {
  qn: string
  label: string
  kind: string
  isSugar?: boolean
  startLine?: number
  endLine?: number
  children?: SourceElement[]
}

export interface SourceFile {
  path: string
  content: string
  lineCount: number
  elements: SourceElement[]
}
