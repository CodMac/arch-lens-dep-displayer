// 指标/元信息/排序接口。
import { get } from './client'
import type { MetaInfo, MetricsMap, SccComponent, SearchResult, TopResult } from '../types'

export function fetchMetrics(dimId: number): Promise<MetricsMap> {
  return get<MetricsMap>(`/dimensions/${dimId}/metrics`)
}

export function fetchScc(dimId: number): Promise<SccComponent[]> {
  return get<SccComponent[]>(`/dimensions/${dimId}/scc`)
}

export function fetchMeta(dimId: number): Promise<MetaInfo> {
  return get<MetaInfo>(`/dimensions/${dimId}/meta`)
}

export function fetchTopBy(dimId: number, by: string, limit = 10): Promise<TopResult> {
  return get<TopResult>(`/dimensions/${dimId}/topby`, { by, limit })
}

export function search(dimId: number, q: string, limit = 20): Promise<SearchResult> {
  return get<SearchResult>(`/dimensions/${dimId}/search`, { q, limit })
}
