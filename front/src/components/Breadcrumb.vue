<script setup lang="ts">
// 层级面包屑：由 dimension 层级序列 + layerStack 驱动。
import { computed } from 'vue'
import { useLayerStackStore } from '../stores/layerStack'

const layerStack = useLayerStackStore()

const crumbs = computed(() =>
  layerStack.breadcrumb.map((entry) => ({
    label: entry.label,
    active: entry.qn === layerStack.current.qn,
  })),
)

function onCrumb(i: number): void {
  layerStack.truncateTo(i)
}
</script>

<template>
  <nav class="breadcrumb">
    <template v-for="(c, i) in crumbs" :key="i">
      <span class="crumb" :class="{ active: c.active }" @click="onCrumb(i)">
        {{ c.label }}
      </span>
      <span v-if="i < crumbs.length - 1" class="sep">/</span>
    </template>
  </nav>
</template>

<style scoped>
.breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 12px;
  height: 40px;
  background: #fff;
  border-bottom: 1px solid #e5e6eb;
  font-size: 13px;
  white-space: nowrap;
  overflow-x: auto;
}

.crumb {
  color: #4e5969;
  cursor: pointer;
}

.crumb:hover {
  color: #165dff;
}

.crumb.active {
  color: #165dff;
  font-weight: 600;
}

.sep {
  color: #c9cdd4;
}
</style>
