<script setup lang="ts">
// L0 仓库总览（需求1）：带规模的全景图。
// 顶层包节点（size=包内类数）+ 包间聚合边（边宽=依赖条数）。点击包进入 L1。
import { ref, onMounted, watch } from 'vue'
import GraphCanvas from '../components/GraphCanvas.vue'
import SourcePanel from '../components/SourcePanel.vue'
import StarPanel from '../components/StarPanel.vue'
import MetricCard from '../components/MetricCard.vue'
import { useDimensionStore } from '../stores/dimension'
import { useLayerStackStore } from '../stores/layerStack'
import { useAnnotationsStore } from '../stores/annotations'
import { fetchOverview } from '../api/nodes'
import { fetchMetrics, fetchScc } from '../api/metrics'
import type { GraphData } from '../types'

const emit = defineEmits<{ (e: 'enter'): void }>()

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()
const annotations = useAnnotationsStore()

const sourcePanelRef = ref<InstanceType<typeof SourcePanel> | null>(null)

const data = ref<GraphData>({ nodes: [], edges: [] })
const loading = ref(false)
const error = ref('')
const metrics = ref<Record<string, number>>({})
const cycleCount = ref(0)

async function load(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  loading.value = true
  error.value = ''
  try {
    data.value = await fetchOverview(dimId)
    metrics.value = await fetchMetrics(dimId)
    const comps = await fetchScc(dimId)
    cycleCount.value = comps.length
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

// 点击包节点 → 进入 L1（下钻到该包）。
function enter(id: string): void {
  const n = data.value.nodes.find((x) => x.id === id)
  if (!n) return
  layerStack.reset()
  layerStack.push({ level: 1, qn: n.id, label: n.label, kind: n.kind })
  emit('enter')
}

// 右键菜单动作：源码定位（需求11）/ 收藏（需求14）。
function onNodeAction(id: string, action: 'drill' | 'fanin' | 'fanout' | 'source' | 'star' | 'unstar'): void {
  if (action === 'source') {
    void sourcePanelRef.value?.openFor(id)
  }
  if (action === 'star' || action === 'unstar') {
    const dimId = dimensionStore.currentId
    if (dimId) void annotations.toggleStar(dimId, id)
  }
}

onMounted(() => {
  void load()
  if (dimensionStore.currentId != null) void annotations.load(dimensionStore.currentId)
})
watch(() => dimensionStore.currentId, (id) => {
  void load()
  if (id != null) void annotations.load(id)
})
</script>

<template>
  <div class="overview">
    <header class="ov-head">
      <h2 class="heading">仓库全景 · {{ dimensionStore.current?.name ?? '…' }}</h2>
      <div class="ov-metrics">
        <MetricCard label="包" :value="metrics.package_total ?? 0" color="#5B8FF9" />
        <MetricCard label="类" :value="metrics.class_total ?? 0" color="#5AD8A6" />
        <MetricCard label="关系" :value="metrics.relation_total ?? 0" color="#F6BD16" />
        <MetricCard label="环" :value="cycleCount" color="#E8684A" />
      </div>
    </header>
    <div class="ov-canvas">
      <div v-if="loading" class="ov-overlay">加载中…</div>
      <div v-else-if="error" class="ov-overlay error">{{ error }}</div>
      <GraphCanvas
        :nodes="data.nodes"
        :edges="data.edges"
        :kind-colors="dimensionStore.kindColors"
        :starred-nodes="annotations.starredSet"
        @node-click="enter"
        @node-action="onNodeAction"
      />
      <SourcePanel ref="sourcePanelRef" />
      <StarPanel />
    </div>
    <p class="hint">点击包节点进入 L1 包内视图 · 边宽 = 包间依赖条数</p>
  </div>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.ov-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px 0;
}

.heading {
  font-size: 18px;
  margin: 0;
}

.ov-metrics {
  display: flex;
  gap: 12px;
}

.ov-canvas {
  position: relative;
  flex: 1;
  min-height: 0;
  margin: 12px;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
}

.ov-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #86909c;
  background: rgba(255, 255, 255, 0.7);
  z-index: 10;
}

.ov-overlay.error {
  color: #e8684a;
}

.hint {
  margin: 0 20px 12px;
  color: #86909c;
  font-size: 12px;
}
</style>
