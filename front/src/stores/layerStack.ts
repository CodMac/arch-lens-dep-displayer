// 层级栈 store：下钻路径（面包屑数据源）。
// 每一项代表用户当前所在的节点路径，栈顶为当前焦点节点。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

/** 栈中的一层。level 为节点所在层级（1=包 2=类 3=成员），qn 为节点 QN。 */
export interface StackEntry {
  level: number
  qn: string
  label: string
  kind: string
}

const ROOT = { level: 0, qn: '', label: '仓库', kind: 'ROOT' }

export const useLayerStackStore = defineStore('layerStack', () => {
  // 栈：ROOT 恒定在底。
  const stack = ref<StackEntry[]>([ROOT])

  const current = computed<StackEntry>(() => stack.value[stack.value.length - 1])
  const breadcrumb = computed<StackEntry[]>(() => stack.value)

  /** 下钻：从当前节点进入子节点（qn, level）。 */
  function push(entry: Omit<StackEntry, 'label' | 'kind'> & Partial<Pick<StackEntry, 'label' | 'kind'>>): void {
    const last = stack.value[stack.value.length - 1]
    // 防止重复 push 同一节点。
    if (last.qn === entry.qn) return
    stack.value.push({
      level: entry.level,
      qn: entry.qn,
      label: entry.label ?? entry.qn.split('.').pop() ?? entry.qn,
      kind: entry.kind ?? '',
    })
  }

  /** 回退到索引处（面包屑点击）。 */
  function truncateTo(index: number): void {
    if (index < 0 || index >= stack.value.length) return
    stack.value = stack.value.slice(0, index + 1)
  }

  /** 重置回根（切语言维度时）。 */
  function reset(): void {
    stack.value = [ROOT]
  }

  /** 整栈恢复（D6 需求20：URL hash 反序列化）。仅 level+qn，label/kind 由 QN 推导。 */
  function hydrate(entries: { level: number; qn: string }[]): void {
    stack.value = [
      ROOT,
      ...entries.map((e) => ({
        level: e.level,
        qn: e.qn,
        label: e.qn.split('.').pop() ?? e.qn,
        kind: '',
      })),
    ]
  }

  return { stack, current, breadcrumb, push, truncateTo, reset, hydrate }
})
