// 与 server REST 契约对齐的类型（docs/middleware.md §6 + frontend.md §4）。

/** 维度层级定义（如 PACKAGE→CLASS→MEMBER）。 */
export interface DimensionLevel {
  level: number
  kind: string
  parent_of: string[]
}

/** 语言维度（GET /api/dimensions）。 */
export interface Dimension {
  id: number
  language: string
  name: string
  levels: DimensionLevel[]
}

/** 图节点。id 为 QN。 */
export interface GraphNode {
  id: string
  label: string
  kind: string
  size: number
}

/** 图边。type 为主类型（兼容旧渲染），types 为完整类型集。 */
export interface GraphEdge {
  source: string
  target: string
  type: string
  /** 完整关系类型集（逗号聚合串已拆分）。 */
  types: string[]
  count: number
}

/** 搜索命中元素。 */
export interface SearchHit {
  qn: string
  kind: string
  name: string
  path?: string
  start_line?: number
  node_qn?: string
  level?: number
}

/** 搜索响应。 */
export interface SearchResult {
  query: string
  hits: SearchHit[]
}

/** 未知/游离元素（meta.unknown_list 项）。 */
export interface UnknownHit {
  qn: string
  kind: string
}

/** 数据元信息（meta 接口）。 */
export interface MetaInfo {
  imported_at?: string
  stats?: Record<string, number>
  isolated_nodes?: number
  isolated_list?: string[]
  unknown_kinds?: number
  unknown_list?: UnknownHit[]
  relation_types?: number
}

/** TOP 排序项（topby 接口）。 */
export interface TopItem {
  qn: string
  label: string
  kind: string
  value: number
}

/** TOP 排序响应。 */
export interface TopResult {
  by: string
  items: TopItem[]
}

/** 图数据（top/children/graph/fanin/fanout 统一返回）。 */
export interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

/** 节点子级（children 接口）。 */
export interface ChildrenResult {
  nodes: GraphNode[]
  edges: []
}

/** 各层指标（metrics 接口，kind→count）。 */
export type MetricsMap = Record<string, number>

/** SCC 强连通分量。 */
export interface SccComponent {
  id: number
  members: string[]
}

/** 源码定位信息（source 接口，需求11）。 */
export interface SourceInfo {
  qn: string
  path: string
  start_line: number
  name: string
}

/** 节点标注/收藏（annotations 接口，需求14）。 */
export interface Annotation {
  dimension_id: number
  qn: string
  starred: boolean
  note: string
  created_at?: string
  updated_at?: string
  kind?: string
  label?: string
}

/** 标注列表响应。 */
export interface AnnotationList {
  items: Annotation[]
}

/** 标注 upsert 响应。 */
export interface AnnotationUpsert {
  qn: string
  starred: boolean
  note: string
}

/** 分析步骤状态（pipeline 步骤快照，供前端进度轮询）。 */
export interface StepResult {
  id: string
  label: string
  status: 'pending' | 'running' | 'done' | 'failed' | 'skipped'
  duration: number
  message?: string
}

/** 数据源（多数据源，GET /api/sources）。 */
export interface Source {
  sid: string
  path: string
  language: string
  status: 'unanalyzed' | 'analyzing' | 'ready' | 'failed'
  active: boolean
  steps?: StepResult[]
  created_at?: string
}

/** 目录浏览项（GET /api/fs/list）。 */
export interface FsItem {
  name: string
  path: string
  is_dir: boolean
}

/** 目录浏览响应。 */
export interface FsListResult {
  path: string
  items: FsItem[]
}

/** 创建数据源响应。 */
export interface CreateSourceResult {
  sid: string
  source: Source
}
