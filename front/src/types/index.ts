// 与后端 REST 契约对齐的类型（数据源 / 维度 / 目录浏览）。

/** 维度层级定义：role ∈ namespace|type|member，kinds = 本层 kind 白名单。 */
export interface DimensionLevel {
  level: number
  role: string
  kinds: string[]
}

/** 语言维度（GET /api/dimensions）。 */
export interface Dimension {
  id: number
  language: string
  qn_separator: string
  /** 展示名（后端不下发，由 language 派生）。 */
  name: string
  levels: DimensionLevel[]
  /** 依赖关系类型白名单（维度配置下发，供切片/环类型按钮使用）。 */
  relation_types: string[]
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
