<script setup lang="ts">
// 全局搜索框（D4-1）：/search 模糊查询，下拉展示命中，点击定位下钻到对应层级。
import { ref, watch } from 'vue'
import { useDimensionStore } from '../stores/dimension'
import { useLayerStackStore } from '../stores/layerStack'
import { search } from '../api/metrics'
import type { SearchHit } from '../types'

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()

const q = ref('')
const hits = ref<SearchHit[]>([])
const open = ref(false)
const searching = ref(false)
const error = ref('')

let timer: ReturnType<typeof setTimeout> | undefined

// 输入防抖后搜索。
watch(q, (v) => {
  clearTimeout(timer)
  if (!v.trim()) {
    hits.value = []
    open.value = false
    return
  }
  timer = setTimeout(() => void doSearch(), 250)
})

async function doSearch(): Promise<void> {
  const dimId = dimensionStore.currentId
  const kw = q.value.trim()
  if (!dimId || !kw) return
  searching.value = true
  error.value = ''
  try {
    const res = await search(dimId, kw, 20)
    hits.value = res.hits ?? []
    open.value = true
  } catch (e) {
    error.value = String(e)
    hits.value = []
  } finally {
    searching.value = false
  }
}

// 点击命中：定位到归属层级节点并下钻。
// 归属 node_qn/level 由 search 返回；无归属（游离元素）时尽力按 QN 前缀定位。
function onPick(h: SearchHit): void {
  open.value = false
  q.value = h.name || h.qn
  const target = h.node_qn || h.qn
  const level = h.level || 2
  layerStack.reset()
  layerStack.push({ level, qn: target, label: h.name || target.split('.').pop() || target, kind: h.kind })
}

function onBlur(): void {
  // 延迟让点击命中先触发。
  setTimeout(() => { open.value = false }, 150)
}

function kindClass(kind: string): string {
  return (kind ?? '').toLowerCase()
}
</script>

<template>
  <div class="search-wrap">
    <input
      v-model="q"
      class="search-input"
      type="text"
      placeholder="搜索类/方法/包…"
      @focus="q.trim() && (open = true)"
      @blur="onBlur"
      @keydown.enter="doSearch"
    />
    <span v-if="searching" class="spin" />
    <ul v-if="open && hits.length" class="search-list">
      <li v-for="h in hits" :key="h.qn" class="item" @mousedown.prevent="onPick(h)">
        <span class="kind" :class="kindClass(h.kind)">{{ h.kind }}</span>
        <span class="name">{{ h.name || h.qn }}</span>
        <span class="path">{{ h.path }}</span>
      </li>
    </ul>
    <div v-if="open && q.trim() && !hits.length && !searching" class="empty">
      {{ error || '无匹配结果' }}
    </div>
  </div>
</template>

<style scoped>
.search-wrap {
  position: relative;
  width: 320px;
}

.search-input {
  width: 100%;
  padding: 6px 10px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
}

.search-input:focus {
  border-color: #165dff;
}

.spin {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  width: 12px;
  height: 12px;
  border: 2px solid #e5e6eb;
  border-top-color: #165dff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: translateY(-50%) rotate(360deg); }
}

.search-list {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  max-height: 360px;
  overflow-y: auto;
  margin: 0;
  padding: 4px 0;
  list-style: none;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  z-index: 50;
}

.item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  cursor: pointer;
  font-size: 12px;
}

.item:hover {
  background: #f2f3f5;
}

.kind {
  flex: none;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  background: #eef2ff;
  color: #165dff;
}

.kind.file, .kind.variable, .kind.anonymous_class {
  background: #fff1e8;
  color: #d25f00;
}

.name {
  color: #1f2329;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.path {
  margin-left: auto;
  color: #86909c;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 120px;
}

.empty {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  padding: 10px;
  text-align: center;
  font-size: 12px;
  color: #86909c;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  z-index: 50;
}
</style>
