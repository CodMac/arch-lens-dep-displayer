// 维度接口（GET /api/dimensions）。
import { get } from './client'
import type { Dimension } from '../types'

export function fetchDimensions(): Promise<Dimension[]> {
  return get<Dimension[]>('/dimensions')
}
