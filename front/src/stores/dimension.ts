// 维度 store：当前语言维度（关系类型集）。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchDimensions } from '../api/dimensions'
import type { Dimension } from '../types'

// 关系类型集（维度配置 relation_types，server 未下发时用默认集合）。
const DEFAULT_RELATION_TYPES = [
  'CALL', 'CREATE', 'EXTEND', 'IMPLEMENT', 'TYPE_ARG', 'CAPTURE',
  'USE', 'ASSIGN', 'CAST', 'PARAMETER', 'RETURN', 'THROW', 'ANNOTATION',
]

export const useDimensionStore = defineStore('dimension', () => {
  const dimensions = ref<Dimension[]>([])
  const currentId = ref<number | null>(null)
  const loading = ref(false)
  const error = ref('')

  const current = computed<Dimension | undefined>(() =>
    dimensions.value.find((d) => d.id === currentId.value),
  )

  /** 依赖关系类型集（维度配置下发；未下发时用默认集合）。 */
  const depRelationTypes = computed<string[]>(() => {
    const fromDim = current.value?.relation_types ?? []
    return fromDim.length ? fromDim : DEFAULT_RELATION_TYPES
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

  /** 切激活源时重置当前维度（新源的维度 id 不同）。 */
  function reset(): void {
    currentId.value = null
    dimensions.value = []
  }

  return {
    dimensions, currentId, loading, error,
    current, depRelationTypes,
    load, reset,
  }
})
