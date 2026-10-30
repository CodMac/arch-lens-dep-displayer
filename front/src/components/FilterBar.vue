<script setup lang="ts">
// 关系类型过滤：选项由维度 relation_types 驱动（含 CONTAIN）。
import { computed } from 'vue'
import { useDimensionStore } from '../stores/dimension'
import { useGraphDataStore } from '../stores/graphData'

const dimensionStore = useDimensionStore()
const graphStore = useGraphDataStore()

const types = computed(() => {
  const present = new Set(graphStore.allTypes)
  // 只展示图里实际出现的类型 + 维度声明的关系类型。
  return dimensionStore.relationTypes.filter((t) => present.has(t))
})
</script>

<template>
  <div class="filter-bar">
    <span class="label">关系类型</span>
    <label v-for="t in types" :key="t" class="chip">
      <input
        type="checkbox"
        :checked="graphStore.typeFilter[t] !== false"
        @change="graphStore.toggleType(t, ($event.target as HTMLInputElement).checked)"
      />
      {{ t }}
    </label>
    <span v-if="!types.length" class="muted">本层无边类型可过滤</span>
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  background: #fff;
  border-bottom: 1px solid #e5e6eb;
  font-size: 12px;
  flex-wrap: wrap;
}

.label {
  color: #86909c;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid #e5e6eb;
  border-radius: 12px;
  cursor: pointer;
  color: #4e5969;
}

.muted {
  color: #c9cdd4;
}
</style>
