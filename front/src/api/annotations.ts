// 节点标注/收藏接口（D6 需求14）。
import { get, put, del } from './client'
import type { AnnotationList, AnnotationUpsert } from '../types'

/** 列出该维度全部标注（GET /annotations）。 */
export function fetchAnnotations(dimId: number): Promise<AnnotationList> {
  return get<AnnotationList>(`/dimensions/${dimId}/annotations`)
}

/** 收藏/备注 upsert（PUT /annotations/{qn}，body 字段可部分更新）。 */
export function upsertAnnotation(
  dimId: number,
  qn: string,
  body: { starred?: boolean; note?: string },
): Promise<AnnotationUpsert> {
  return put<AnnotationUpsert>(`/dimensions/${dimId}/annotations/${encodeURIComponent(qn)}`, body)
}

/** 删除标注（DELETE /annotations/{qn}，取消收藏并清除备注）。 */
export function deleteAnnotation(dimId: number, qn: string): Promise<{ ok: boolean; qn: string }> {
  return del<{ ok: boolean; qn: string }>(`/dimensions/${dimId}/annotations/${encodeURIComponent(qn)}`)
}
