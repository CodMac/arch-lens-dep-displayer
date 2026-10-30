// 维度 store：当前语言维度（层级序列、关系类型集、kind 颜色映射）。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchDimensions } from '../api/dimensions'
import type { Dimension } from '../types'

// 关系类型集（维度配置 relation_types，server 未下发时用默认集合）。
export const DEFAULT_RELATION_TYPES = [
  'CALL', 'CREATE', 'EXTEND', 'IMPLEMENT', 'TYPE_ARG', 'CAPTURE',
  'USE', 'ASSIGN', 'CAST', 'PARAMETER', 'RETURN', 'THROW', 'ANNOTATION',
]

export const CONTAIN_TYPE = 'CONTAIN'

export const useDimensionStore = defineStore('dimension', () => {
  const dimensions = ref<Dimension[]>([])
  const currentId = ref<number | null>(null)
  const loading = ref(false)
  const error = ref('')

  const current = computed<Dimension | undefined>(() =>
    dimensions.value.find((d) => d.id === currentId.value),
  )

  /** 关系类型集（含 CONTAIN 聚合边）。 */
  const relationTypes = computed<string[]>(() => {
    const base = current.value ? DEFAULT_RELATION_TYPES : []
    return Array.from(new Set([...base, CONTAIN_TYPE]))
  })

  /** 当前维度层级序列（level 升序）。 */
  const levels = computed(() => {
    const ls = current.value?.levels ?? []
    return [...ls].sort((a, b) => a.level - b.level)
  })

  /** 节点 kind → 颜色映射（维度驱动，默认按层级 kind）。 */
  const kindColors = computed<Record<string, string>>(() => {
    const palette = [
      '#5B8FF9', '#5AD8A6', '#F6BD16', '#E8684A', '#6DC8EC',
      '#9270CA', '#FF9D4D', '#269A99', '#FF99C3', '#A0A0A0',
    ]
    const m: Record<string, string> = {}
    levels.value.forEach((lv, i) => {
      m[lv.kind] = palette[i % palette.length]
      for (const k of lv.parent_of) {
        m[k] = palette[i % palette.length]
      }
    })
    m.CONTAIN = palette[8]
    m.UNKNOWN = palette[9]
    return m
  })

  async function load(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      dimensions.value = await fetchDimensions()
      if (dimensions.value.length && currentId.value == null) {
        currentId.value = dimensions.value[0].id
      }
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  function select(id: number): void {
    currentId.value = id
  }

  /** 切激活源时重置当前维度（新源的维度 id 不同）。 */
  function reset(): void {
    currentId.value = null
    dimensions.value = []
  }

  return {
    dimensions, currentId, loading, error,
    current, relationTypes, levels, kindColors,
    load, select, reset,
  }
})
