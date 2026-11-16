// 维度接口（GET /api/dimensions）。
import { get } from './client'
import type { Dimension } from '../types'

/** 后端不下发展示名：由 language 派生（java → Java）。 */
function displayName(language: string): string {
  if (!language) return '未知语言'
  return language.charAt(0).toUpperCase() + language.slice(1)
}

export async function fetchDimensions(): Promise<Dimension[]> {
  const list = await get<Dimension[]>('/dimensions')
  return (list ?? []).map((d) => ({ ...d, name: d.name || displayName(d.language) }))
}
