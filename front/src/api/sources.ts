// 数据源 / 目录浏览 API 封装（多数据源 + 前端选源码目录）。
import { get, post } from './client'
import type { Source, FsListResult, CreateSourceResult } from '../types'

/** 列出全部数据源。 */
export async function fetchSources(): Promise<Source[]> {
  const res = await get<{ items: Source[] }>('/sources')
  return res.items
}

/** 创建数据源（启动后台分析），返回 sid。 */
export async function createSource(path: string, language?: string): Promise<CreateSourceResult> {
  return post<CreateSourceResult>('/sources', { path, language })
}

/** 获取某数据源状态（步骤进度，供轮询）。 */
export async function fetchSource(sid: string): Promise<Source> {
  return get<Source>(`/sources/${encodeURIComponent(sid)}`)
}

/** 切换激活数据源。 */
export async function activateSource(sid: string): Promise<void> {
  await post(`/sources/${encodeURIComponent(sid)}/activate`)
}

/** 目录浏览。 */
export async function fsList(path?: string): Promise<FsListResult> {
  return get<FsListResult>('/fs/list', { path })
}
