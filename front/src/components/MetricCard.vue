<script setup lang="ts">
// 指标卡：节点/边/环等单值展示。
defineProps<{
  label: string
  value: number | string
  color?: string
}>()
</script>

<template>
  <div class="metric-card">
    <div class="value" :style="{ color: color ?? '#1f2329' }">{{ value }}</div>
    <div class="label">{{ label }}</div>
  </div>
</template>

<style scoped>
.metric-card {
  flex: 1;
  min-width: 120px;
  padding: 14px 16px;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #e5e6eb;
}

.value {
  font-size: 26px;
  font-weight: 600;
  line-height: 1.2;
}

.label {
  margin-top: 6px;
  font-size: 12px;
  color: #86909c;
}
</style>
