<script setup lang="ts">
// L0 仓库总览（需求1）：Combo 嵌套气泡的全景图。
// 每个包是一个气泡（Combo），子包气泡嵌套进父包气泡（com 包含 com.aaa），
// 叶子包（无子包）是父包气泡内的小圆点，点击下钻进类层。
// 依赖边在全量包间显示；Combo 收起时 G6 collapse-expand-combo 自动把内部边卷到气泡、外部边保留。
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
import type { GraphNode, GraphEdge, ComboData } from '../types'

const emit = defineEmits<{ (e: 'enter'): void }>()

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()
const annotations = useAnnotationsStore()

const sourcePanelRef = ref<InstanceType<typeof SourcePanel> | null>(null)

const loading = ref(false)
const error = ref('')
const metrics = ref<Record<string, number>>({})
const cycleCount = ref(0)

// 全量包气泡（Combo）与叶子包节点、包间依赖边。
const combos = ref<ComboData[]>([])
const nodes = ref<GraphNode[]>([])
const edges = ref<GraphEdge[]>([])

// ---- 数据加载 ----

async function load(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  loading.value = true
  error.value = ''
  try {
    const ov = await fetchOverview(dimId)
    // ov.nodes = 全部包；非叶子包 → Combo（气泡），叶子包 → 父包气泡内节点。
    const pkgParent = new Map<string, string>()
    for (const t of ov.tree) pkgParent.set(t.qn, t.parent)

    const comboList: ComboData[] = []
    const nodeList: GraphNode[] = []
    for (const n of ov.nodes) {
      const parent = pkgParent.get(n.id) ?? ''
      if ((n.children ?? 0) > 0) {
        comboList.push({
          id: n.id,
          label: n.label,
          parent,
          size: n.size,
          children: n.children ?? 0,
          depth: n.depth ?? 0,
        })
      } else {
        nodeList.push({ ...n, parent })
      }
    }
    combos.value = comboList
    nodes.value = nodeList
    edges.value = ov.edges

    metrics.value = await fetchMetrics(dimId)
    const comps = await fetchScc(dimId)
    cycleCount.value = comps.length
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

// ---- 点击交互 ----

// 点击叶子包（圆点）→ 下钻进类层；Combo 气泡的展开/收起由 G6 双击驱动。
function onNodeClick(id: string): void {
  const n = nodes.value.find((x) => x.id === id)
  if (!n) return
  layerStack.reset()
  layerStack.push({ level: 1, qn: n.id, label: n.label, kind: n.kind })
  emit('enter')
}

// 右键菜单动作：源码定位 / 收藏。
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
        :nodes="nodes"
        :edges="edges"
        :combos="combos"
        :kind-colors="dimensionStore.kindColors"
        :starred-nodes="annotations.starredSet"
        @node-click="onNodeClick"
        @node-action="onNodeAction"
      />
      <SourcePanel ref="sourcePanelRef" />
      <StarPanel />
    </div>
    <p class="hint">气泡 = 包（单击展开/收起） · 小圆点 = 叶子包（点击进入类层） · 边宽 = 包间依赖条数</p>
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
