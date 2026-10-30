// 节点下钻/图/扇入扇出接口。
import { get } from './client'
import type { ChildrenResult, GraphData, SourceInfo } from '../types'

/** 顶层节点（GET /top?level=N）。 */
export function fetchTop(dimId: number, level = 1): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/top`, { level })
}

/** 子级（GET /nodes/{qn}/children）。 */
export function fetchChildren(dimId: number, qn: string): Promise<ChildrenResult> {
  return get<ChildrenResult>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/children`)
}

/** 邻接子图（GET /nodes/{qn}/graph?types=...）。 */
export function fetchGraph(dimId: number, qn: string, types?: string[]): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/graph`, {
    types: types && types.length ? types.join(',') : undefined,
  })
}

/** 扇入（GET /nodes/{qn}/fanin?depth=N）。 */
export function fetchFanin(dimId: number, qn: string, depth = 2): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/fanin`, { depth })
}

/** 扇出（GET /nodes/{qn}/fanout?depth=N）。 */
export function fetchFanout(dimId: number, qn: string, depth = 2): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/fanout`, { depth })
}

/**
 * 前向可达子图（GET /nodes/{qn}/fanout?depth=1，仅邻接）。
 * D3-2 路径搜索用：每次取 qn 的直接扇出邻接边。
 */
export function fetchFanoutAdj(dimId: number, qn: string): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/fanout`, { depth: 1 })
}

/** 首页全景（GET /overview）：顶层包 + 包间聚合边。需求1。 */
export function fetchOverview(dimId: number): Promise<GraphData> {
  return get<GraphData>(`/dimensions/${dimId}/overview`)
}

/** 源码定位（GET /nodes/{qn}/source）。需求11。 */
export function fetchSource(dimId: number, qn: string): Promise<SourceInfo> {
  return get<SourceInfo>(`/dimensions/${dimId}/nodes/${encodeURIComponent(qn)}/source`)
}
