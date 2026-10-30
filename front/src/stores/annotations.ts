// 节点标注/收藏 store（D6 需求14）：按维度隔离，提供星标集合与收藏列表。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchAnnotations, upsertAnnotation, deleteAnnotation } from '../api/annotations'
import type { Annotation } from '../types'

function lastSeg(qn: string): string {
  const i = qn.lastIndexOf('.')
  return i >= 0 ? qn.slice(i + 1) : qn
}

export const useAnnotationsStore = defineStore('annotations', () => {
  const items = ref<Annotation[]>([])
  const loading = ref(false)
  const error = ref('')

  /** QN → 标注 映射。 */
  const byQn = computed<Record<string, Annotation>>(() => {
    const m: Record<string, Annotation> = {}
    for (const a of items.value) m[a.qn] = a
    return m
  })

  /** 仅收藏项（列表展示）。 */
  const starredList = computed<Annotation[]>(() =>
    items.value.filter((a) => a.starred).map((a) => ({ ...a, label: a.label ?? lastSeg(a.qn) })),
  )

  /** 收藏 QN 集合（画布星标样式用）。 */
  const starredSet = computed<Set<string>>(() => new Set(items.value.filter((a) => a.starred).map((a) => a.qn)))

  /** 加载该维度全部标注。 */
  async function load(dimId: number): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await fetchAnnotations(dimId)
      items.value = res.items
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  /** 收藏/取消收藏（已收藏则删除，否则 upsert starred:true）。 */
  async function toggleStar(dimId: number, qn: string): Promise<boolean> {
    try {
      if (byQn.value[qn]?.starred) {
        await deleteAnnotation(dimId, qn)
        items.value = items.value.filter((a) => a.qn !== qn)
        return false
      }
      const res = await upsertAnnotation(dimId, qn, { starred: true })
      upsertLocal(dimId, qn, res)
      return true
    } catch (e) {
      error.value = String(e)
      return byQn.value[qn]?.starred ?? false
    }
  }

  /** 保存备注（upsert note；若未收藏只存备注，不进星标列表）。 */
  async function setNote(dimId: number, qn: string, note: string): Promise<void> {
    try {
      const res = await upsertAnnotation(dimId, qn, { note })
      upsertLocal(dimId, qn, res)
    } catch (e) {
      error.value = String(e)
    }
  }

  /** 本地合并 upsert 结果（保留已存在的 kind/label）。 */
  function upsertLocal(dimId: number, qn: string, res: { starred: boolean; note: string }): void {
    const exist = byQn.value[qn]
    if (exist) {
      exist.starred = res.starred
      exist.note = res.note
      exist.updated_at = new Date().toISOString()
    } else {
      items.value = [...items.value, {
        dimension_id: dimId,
        qn,
        starred: res.starred,
        note: res.note,
        label: lastSeg(qn),
      }]
    }
  }

  function reset(): void {
    items.value = []
    error.value = ''
  }

  return { items, loading, error, byQn, starredList, starredSet, load, toggleStar, setNote, reset }
})
