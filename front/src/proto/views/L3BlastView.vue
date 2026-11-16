<script setup lang="ts">
// L2 影响面（11.19 重写）：类/包级扇入扇出 + 方法级 Blast Radius 两档。
//  - 默认（无 method）：类/包级双向辐射。左翼 = 谁依赖我（扇入），右翼 = 我依赖谁（扇出）。
//  - 带 method 参数：方法级调用点辐射。左翼 = 谁调用我，右翼 = 我调用谁。
// 蝴蝶布局：中心置中，上游左翼、下游右翼，按 BFS 层数横向递进、同层纵向均分。
import { ref, computed, onMounted, watch } from 'vue'
import ProtoGraphCanvas from '../components/ProtoGraphCanvas.vue'
import { useProtoStore } from '../protoStore'
import { useDimensionStore } from '../../stores/dimension'
import { applyButterflyLayout } from '../fanGraph'
import type { FanPayload, FanNode, FanNodeStat } from '../fanGraph'
import type { ActualEdge } from '../types/model'

const props = defineProps<{ qn: string; method?: string }>()
const store = useProtoStore()
const emit = defineEmits<{
  (e: 'go-source', qn: string): void
  (e: 'back'): void
  /** 类/包模式点击节点 → 以该节点为新中心。 */
  (e: 'recenter', qn: string): void
  /** 11.20：方法级调用点 → L3 源码（带行号高亮）。 */
  (e: 'go-source-line', srcQn: string, dstQn: string, line: number): void
}>()

const dimensionStore = useDimensionStore()
/** 关系类型全集（维度配置下发，避免多语言硬编码）。 */
const ALL_TYPES = computed(() => dimensionStore.depRelationTypes)

const loading = ref(true)
/** 服务端 /fan 结果（BFS/统计/节点明细全在服务端）。 */
const serverPayload = ref<FanPayload | null>(null)

/** 深度（BFS 层数）。 */
const depth = ref(2)
const maxDepth = ref(4)
/** 关系切片（全选 = 不过滤）。 */
const enabledTypes = ref<Set<string>>(new Set(dimensionStore.depRelationTypes))
/** 隐藏实现细节（lambda / 匿名类 / 代码块，后端 is_impl_detail）。 */
const hideImplDetail = ref(false)
/** 类/包粒度（method 模式下不适用）。 */
const granularity = ref<'package' | 'type'>('type')

/** 目标是否为方法（带 method 参数）。 */
const isMethodMode = computed(() => !!props.method)
/** 实际分析对象：方法模式用 method，否则用 qn。 */
const target = computed(() => props.method ?? props.qn)

/** 目标是否是类（含大写段）——决定默认粒度。 */
function looksLikeType(qn: string): boolean {
  return qn.split('.').some((s) => /^[A-Z]/.test(s))
}

/** 完整图：服务端 /fan（BFS/统计/节点明细全在服务端）。 */
const fullPayload = computed<FanPayload | null>(() => {
  if (loading.value) return null
  return serverPayload.value
})

/** 实现细节过滤 + 蝴蝶布局（每次重算坐标，避免节点复用旧坐标）。 */
const payload = computed<FanPayload | null>(() => {
  const p = fullPayload.value
  if (!p) return null
  let nodes = p.nodes
  let edges = p.edges
  if (hideImplDetail.value) {
    const hidden = new Set(nodes.filter((n) => n.isImplDetail).map((n) => n.id))
    nodes = nodes.filter((n) => !hidden.has(n.id))
    edges = edges.filter((e) => !hidden.has(e.source) && !hidden.has(e.target))
  }
  const next: FanPayload = { ...p, nodes, edges }
  applyButterflyLayout(next)
  return next
})

const nodes = computed<FanNode[]>(() => payload.value?.nodes ?? [])
const edges = computed(() => payload.value?.edges ?? [])
const stats = computed(() => payload.value?.stats ?? null)
const relationTypes = computed(() => payload.value?.relationTypes ?? ALL_TYPES.value)

async function load(): Promise<void> {
  loading.value = true
  serverPayload.value = null
  if (!isMethodMode.value) {
    // 目标不是类 → 默认包粒度。
    granularity.value = looksLikeType(props.qn) ? 'type' : 'package'
  }
  await refreshPayload()
  loading.value = false
}

/** 按当前粒度/深度/切片请求 /fan。 */
async function refreshPayload(): Promise<void> {
  const types = enabledTypes.value.size === ALL_TYPES.value.length ? [] : [...enabledTypes.value]
  serverPayload.value = await store.loadFan(target.value, {
    granularity: isMethodMode.value ? 'method' : granularity.value,
    depth: depth.value,
    types,
    hideImplDetail: false, // 过滤在前端做（与布局共用一次节点集）
  })
}

onMounted(() => void load())
watch(() => [props.qn, props.method], () => void load())
watch([depth, granularity, enabledTypes], () => void refreshPayload())

function toggleType(t: string): void {
  const s = new Set(enabledTypes.value)
  if (s.has(t)) s.delete(t)
  else s.add(t)
  enabledTypes.value = s
}

function setGranularity(g: 'package' | 'type'): void {
  granularity.value = g
}

/** 风险分档：直接依赖 + 跨包越界（不随深度滑块变化，避免「风险」随视图漂移）。 */
const risk = computed(() => {
  const s = stats.value
  if (!s) return { score: 0, level: 'low' as const }
  const score = s.directUp + s.directDown + s.crossPkg * 2
  const level = score < 10 ? ('low' as const) : score <= 30 ? ('mid' as const) : ('high' as const)
  return { score, level }
})
const riskColor = computed(() =>
  risk.value.level === 'low' ? '#5AD8A6' : risk.value.level === 'mid' ? '#F6BD16' : '#E8684A',
)

const unitLabel = computed(() => {
  if (isMethodMode.value) return '方法'
  return granularity.value === 'package' ? '包' : '类'
})

// ---- 节点详情（11.20）：点击节点 → 扇入扇出明细 + 直接邻居列表 ----
const selectedNode = ref<string | null>(null)
/** 邻居列表展示条数上限（超出折叠）。 */
const NEIGHBOR_LIMIT = 12
const showAllIn = ref(false)
const showAllOut = ref(false)

const selectedStat = computed<FanNodeStat | null>(() => {
  const id = selectedNode.value
  const p = payload.value
  if (!id || !p) return null
  return p.nodeStats[id] ?? null
})

const selectedNodeInfo = computed<FanNode | null>(() => {
  const id = selectedNode.value
  if (!id) return null
  return nodes.value.find((n) => n.id === id) ?? null
})

/** 邻居 QN → 展示名（类/包取短名，方法取方法签名）。 */
function shortName(qn: string): string {
  if (isMethodMode.value) {
    const segs = qn.split('.')
    let i = 0
    while (i < segs.length && !/^[A-Z]/.test(segs[i]!)) i++
    return qn.slice(segs.slice(0, i + 1).join('.').length + 1) || (segs[segs.length - 1] ?? qn)
  }
  return qn.split('.').pop() ?? qn
}

const inNeighbors = computed(() => {
  const s = selectedStat.value
  if (!s) return []
  return showAllIn.value ? s.inNeighbors : s.inNeighbors.slice(0, NEIGHBOR_LIMIT)
})
const outNeighbors = computed(() => {
  const s = selectedStat.value
  if (!s) return []
  return showAllOut.value ? s.outNeighbors : s.outNeighbors.slice(0, NEIGHBOR_LIMIT)
})

function onNodeClick(id: string): void {
  selectedEdge.value = null
  selectedNode.value = id
  showAllIn.value = false
  showAllOut.value = false
}

/** 邻居点击 → 以它为新中心（类/包模式）或跳方法级（方法模式）。 */
function onNeighborClick(id: string): void {
  if (isMethodMode.value) {
    emit('go-source', id)
    return
  }
  selectedNode.value = null
  emit('recenter', id)
}

// ---- 边详情（11.20）：点击边 → 依赖明细（方法级可展开调用点） ----
const selectedEdge = ref<{ source: string; target: string; types: string[]; count: number } | null>(null)
/** 方法级：该边的调用点明细。 */
const callDetails = ref<ActualEdge[] | null>(null)
const callLoading = ref(false)

async function onEdgeClick(source: string, target: string): Promise<void> {
  selectedNode.value = null
  const hit = edges.value.find((e) => e.source === source && e.target === target)
  selectedEdge.value = {
    source,
    target,
    types: hit?.types ?? (hit?.type ? [hit.type] : []),
    count: hit?.count ?? 0,
  }
  callDetails.value = null
  if (!isMethodMode.value) return
  callLoading.value = true
  const detail = await store.loadMethodCallDetails(source, target)
  // 期间用户可能已点别的边。
  if (selectedEdge.value?.source !== source || selectedEdge.value?.target !== target) return
  callDetails.value = detail
  callLoading.value = false
}

/** 调用点 → L3 源码（带行号高亮）。 */
function openCallSource(d: ActualEdge): void {
  emit('go-source-line', d.sourceQn, d.targetQn, d.line)
}

function openSource(): void {
  emit('go-source', target.value)
}

function closeDetails(): void {
  selectedNode.value = null
  selectedEdge.value = null
  callDetails.value = null
}
</script>

<template>
  <div class="l2">
    <header class="l2-head">
      <button class="back-btn" @click="emit('back')">‹ 返回 L1</button>
      <div class="l2-title">
        <h3>L2 · 影响面（{{ isMethodMode ? '方法级辐射' : unitLabel + '级扇入扇出' }}）</h3>
        <span class="l2-sub">
          左翼 = {{ isMethodMode ? '谁调用我' : '谁依赖我' }} · 右翼 = {{ isMethodMode ? '我调用谁' : '我依赖谁' }}
        </span>
      </div>
    </header>

    <div v-if="payload" class="l2-body">
      <!-- 控制条 -->
      <div class="l2-controls">
        <div v-if="!isMethodMode" class="ctl">
          <span class="ctl-label">粒度</span>
          <button class="gran-btn" :class="{ on: granularity === 'package' }" @click="setGranularity('package')">包</button>
          <button class="gran-btn" :class="{ on: granularity === 'type' }" @click="setGranularity('type')">类</button>
        </div>
        <div class="ctl">
          <span class="ctl-label">辐射深度</span>
          <input v-model.number="depth" type="range" min="1" :max="maxDepth" />
          <span class="ctl-val">{{ depth }} 层</span>
        </div>
        <label v-if="isMethodMode" class="ctl sugar">
          <input v-model="hideImplDetail" type="checkbox" /> 隐藏实现细节（lambda / 匿名类 / 代码块）
        </label>
      </div>

      <!-- 关系切片（按数据里实际出现的类型生成，避免无效按钮） -->
      <div class="l2-types">
        <span class="ctl-label">关系切片</span>
        <button
          v-for="t in relationTypes"
          :key="t"
          class="type-chip"
          :class="{ on: enabledTypes.has(t) }"
          @click="toggleType(t)"
        >{{ t }}</button>
      </div>

      <!-- 中心节点卡片 -->
      <div class="l2-center">
        <span class="center-kind">{{ isMethodMode ? 'METHOD' : unitLabel.toUpperCase() }}</span>
        <span class="center-qn" :title="target">{{ target }}</span>
        <button class="center-src" @click="openSource">L3 源码</button>
      </div>

      <div class="l2-canvas">
        <ProtoGraphCanvas
          :nodes="nodes"
          :edges="edges"
          :fixed-layout="true"
          :kind-colors="{
            PACKAGE: '#5B8FF9', CLASS: '#5AD8A6', INTERFACE: '#6DC8EC',
            ENUM: '#F6BD16', ANNOTATION: '#FF9D4D',
            METHOD: '#5AD8A6', LAMBDA: '#FF9D4D', SCOPE_BLOCK: '#A0A0A0',
            ANONYMOUS_CLASS: '#9270CA', UNKNOWN: '#A0A0A0',
          }"
          @node-click="onNodeClick"
          @edge-click="onEdgeClick"
        />

        <!-- 节点详情（11.20）：扇入扇出明细 + 直接邻居 -->
        <div v-if="selectedStat" class="l2-detail">
          <div class="dp-head">
            <span class="dp-title">{{ isMethodMode ? '方法详情' : unitLabel + '详情' }}</span>
            <button class="dp-close" title="关闭" @click="closeDetails">✕</button>
          </div>
          <div class="dp-body">
            <div class="dp-name" :title="selectedStat.id">{{ shortName(selectedStat.id) }}</div>
            <div class="dp-qn" :title="selectedStat.id">{{ selectedStat.id }}</div>

            <div class="dp-sec">扇入 / 扇出（引用次数）</div>
            <div class="dp-table">
              <div class="dp-tr"><span>扇入</span><b class="up">{{ selectedStat.fanIn }} 次 / {{ selectedStat.inNeighbors.length }} 个</b></div>
              <div class="dp-tr"><span>扇出</span><b class="down">{{ selectedStat.fanOut }} 次 / {{ selectedStat.outNeighbors.length }} 个</b></div>
              <div v-if="selectedNodeInfo" class="dp-tr"><span>位置</span><b>{{ selectedNodeInfo.side === 'center' ? '中心' : selectedNodeInfo.side === 'up' ? '左翼（上游）' : '右翼（下游）' }} · 第 {{ selectedNodeInfo.depth }} 层</b></div>
            </div>

            <div class="dp-sec">
              直接上游（谁依赖我）
              <span class="dp-count">{{ selectedStat.inNeighbors.length }}</span>
            </div>
            <div v-if="!selectedStat.inNeighbors.length" class="dp-empty">无直接上游</div>
            <div v-for="nb in inNeighbors" :key="'in-' + nb.id" class="dp-nb" :title="nb.id" @click="onNeighborClick(nb.id)">
              <span class="nb-name">{{ shortName(nb.id) }}</span>
              <span class="nb-types">{{ nb.types.join(', ') }}</span>
              <span class="nb-count">×{{ nb.count }}</span>
            </div>
            <button
              v-if="selectedStat.inNeighbors.length > NEIGHBOR_LIMIT"
              class="dp-more"
              @click="showAllIn = !showAllIn"
            >{{ showAllIn ? '收起' : `展开全部 ${selectedStat.inNeighbors.length} 个` }}</button>

            <div class="dp-sec">
              直接下游（我依赖谁）
              <span class="dp-count">{{ selectedStat.outNeighbors.length }}</span>
            </div>
            <div v-if="!selectedStat.outNeighbors.length" class="dp-empty">无直接下游</div>
            <div v-for="nb in outNeighbors" :key="'out-' + nb.id" class="dp-nb" :title="nb.id" @click="onNeighborClick(nb.id)">
              <span class="nb-name">{{ shortName(nb.id) }}</span>
              <span class="nb-types">{{ nb.types.join(', ') }}</span>
              <span class="nb-count">×{{ nb.count }}</span>
            </div>
            <button
              v-if="selectedStat.outNeighbors.length > NEIGHBOR_LIMIT"
              class="dp-more"
              @click="showAllOut = !showAllOut"
            >{{ showAllOut ? '收起' : `展开全部 ${selectedStat.outNeighbors.length} 个` }}</button>
          </div>
          <div class="dp-foot">
            <button v-if="!isMethodMode && selectedStat.id !== target" class="dp-btn primary" @click="onNeighborClick(selectedStat.id)">以此为中心</button>
            <button class="dp-btn" @click="emit('go-source', selectedStat.id)">L3 源码</button>
            <button class="dp-btn" @click="closeDetails">收起</button>
          </div>
        </div>

        <!-- 边详情（11.20）：依赖明细；方法级展开调用点 -->
        <div v-if="selectedEdge" class="l2-detail edge-detail">
          <div class="dp-head">
            <span class="dp-title">依赖详情</span>
            <button class="dp-close" title="关闭" @click="closeDetails">✕</button>
          </div>
          <div class="dp-body">
            <div class="dp-sec">依赖</div>
            <div class="dp-table">
              <div class="dp-tr"><span>源</span><b class="dp-link" :title="selectedEdge.source" @click="onNeighborClick(selectedEdge.source)">{{ shortName(selectedEdge.source) }}</b></div>
              <div class="dp-tr"><span>目标</span><b class="dp-link" :title="selectedEdge.target" @click="onNeighborClick(selectedEdge.target)">{{ shortName(selectedEdge.target) }}</b></div>
              <div class="dp-tr"><span>次数</span><b>{{ selectedEdge.count }}</b></div>
              <div class="dp-tr"><span>类型</span><b>{{ selectedEdge.types.length ? selectedEdge.types.join(', ') : '—' }}</b></div>
            </div>
            <div class="dp-qn" :title="selectedEdge.source + ' → ' + selectedEdge.target">{{ selectedEdge.source }} → {{ selectedEdge.target }}</div>

            <template v-if="isMethodMode">
              <div class="dp-sec">调用点明细<span class="dp-count">{{ callDetails?.length ?? 0 }}</span></div>
              <div v-if="callLoading" class="dp-empty">加载中…</div>
              <div v-else-if="!callDetails?.length" class="dp-empty">无调用点明细（类型级关系）</div>
              <div v-for="(d, i) in callDetails" :key="i" class="dp-call" :title="d.sourceQn + ' → ' + d.targetQn">
                <span class="call-loc">{{ d.file.split('/').pop() }}:{{ d.line }}</span>
                <span class="call-type">{{ d.type }}</span>
                <span class="call-count">×{{ d.count }}</span>
                <button v-if="d.line > 0" class="call-src" @click="openCallSource(d)">源码</button>
              </div>
            </template>
          </div>
          <div class="dp-foot">
            <button class="dp-btn" @click="closeDetails">收起</button>
          </div>
        </div>
      </div>

      <!-- 风险看板 -->
      <div class="l2-score" :style="{ borderColor: riskColor }">
        <div class="score-main">
          <span class="score-num" :style="{ color: riskColor }">{{ risk.score }}</span>
          <span class="score-lv" :style="{ background: riskColor }">
            {{ risk.level === 'low' ? '低风险' : risk.level === 'mid' ? '中风险' : '高风险' }}
          </span>
        </div>
        <div class="score-break">
          <span>左翼 {{ unitLabel }} <b class="up">{{ stats?.upCount ?? 0 }}</b>（直接 {{ stats?.directUp ?? 0 }}）</span>
          <span>右翼 {{ unitLabel }} <b class="down">{{ stats?.downCount ?? 0 }}</b>（直接 {{ stats?.directDown ?? 0 }}）</span>
          <span>跨包越界 <b class="warn">{{ stats?.crossPkg ?? 0 }}</b></span>
          <span>扇入 <b>{{ stats?.fanIn ?? 0 }}</b> 次 / 扇出 <b>{{ stats?.fanOut ?? 0 }}</b> 次</span>
          <span class="score-formula">Score = 直接左翼 + 直接右翼 + 2×跨包越界</span>
        </div>
      </div>
    </div>

    <div v-else class="l2-loading">加载中…</div>

    <p class="l2-hint">
      蝴蝶布局：中心置中，左翼上游 / 右翼下游 · 深度滑块控制 BFS 层数 · 关系切片按实际出现的类型过滤
      <span v-if="isMethodMode"> · 点击任意方法节点进入 L3 源码</span>
      <span v-else> · 点击任意{{ unitLabel }}节点以它为中心重算</span>
    </p>
  </div>
</template>

<style scoped>
.l2 {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.l2-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 20px 0;
}
.back-btn {
  padding: 4px 12px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
  color: #4e5969;
}
.l2-title h3 {
  margin: 0;
  font-size: 16px;
}
.l2-sub {
  color: #86909c;
  font-size: 12px;
}
.l2-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 12px 20px 0;
}
.l2-controls {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
  padding: 8px 12px;
  background: #f7f8fa;
  border-radius: 8px;
}
.ctl {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ctl-label {
  font-size: 13px;
  color: #4e5969;
  white-space: nowrap;
}
.ctl-val {
  font-size: 13px;
  color: #165dff;
  min-width: 36px;
}
.ctl.sugar {
  font-size: 13px;
  color: #4e5969;
}
.gran-btn {
  padding: 2px 10px;
  border: 1px solid #d5dbe3;
  border-radius: 10px;
  background: #fff;
  font-size: 12px;
  color: #86909c;
  cursor: pointer;
}
.gran-btn.on {
  background: #e8f3ff;
  border-color: #165dff;
  color: #165dff;
}
.l2-types {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 8px 12px 0;
}
.type-chip {
  padding: 2px 8px;
  border: 1px solid #d5dbe3;
  border-radius: 10px;
  background: #fff;
  font-size: 11px;
  color: #86909c;
  cursor: pointer;
}
.type-chip.on {
  background: #e8f3ff;
  border-color: #165dff;
  color: #165dff;
}
.l2-center {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 8px;
  border-bottom: 1px solid #f2f3f5;
}
.center-kind {
  padding: 2px 8px;
  border-radius: 4px;
  background: #e8684a;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
}
.center-qn {
  font-family: monospace;
  font-size: 13px;
  color: #1f2329;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.center-src {
  padding: 2px 8px;
  border: 1px solid #d5dbe3;
  border-radius: 5px;
  background: #fff;
  font-size: 11px;
  color: #4e5969;
  cursor: pointer;
}
.l2-canvas {
  position: relative;
  flex: 1;
  min-height: 0;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
  border: 1px solid #e5e6eb;
}

/* 节点/边详情面板（11.20）：画布右上浮层。 */
.l2-detail {
  position: absolute;
  top: 12px;
  right: 12px;
  width: 340px;
  max-height: calc(100% - 24px);
  display: flex;
  flex-direction: column;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
  z-index: 20;
}
.l2-detail.edge-detail {
  width: 380px;
}
.dp-head {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid #eef1f4;
}
.dp-title {
  font-size: 13px;
  font-weight: 600;
  color: #1f2329;
}
.dp-close {
  margin-left: auto;
  border: none;
  background: none;
  color: #86909c;
  cursor: pointer;
  font-size: 13px;
}
.dp-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 10px 12px;
  font-size: 12px;
}
.dp-name {
  font-size: 14px;
  font-weight: 600;
  color: #1f2329;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dp-qn {
  margin-top: 2px;
  font-family: monospace;
  font-size: 10px;
  color: #86909c;
  word-break: break-all;
}
.dp-sec {
  margin-top: 12px;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  color: #4e5969;
}
.dp-count {
  padding: 0 5px;
  border-radius: 8px;
  background: #f2f3f5;
  color: #86909c;
  font-size: 10px;
  font-weight: 400;
}
.dp-table {
  border: 1px solid #eef1f4;
  border-radius: 6px;
  overflow: hidden;
}
.dp-tr {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  font-size: 11px;
}
.dp-tr + .dp-tr {
  border-top: 1px solid #f5f6f8;
}
.dp-tr span {
  color: #86909c;
  flex-shrink: 0;
}
.dp-tr b {
  color: #1f2329;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dp-tr b.up {
  color: #165dff;
}
.dp-tr b.down {
  color: #2f9e6e;
}
.dp-link {
  cursor: pointer;
  color: #165dff !important;
}
.dp-link:hover {
  text-decoration: underline;
}
.dp-empty {
  padding: 6px 4px;
  color: #a0a8b3;
  font-size: 11px;
}
.dp-nb {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 4px;
  border-radius: 4px;
  cursor: pointer;
}
.dp-nb:hover {
  background: #f2f5fa;
}
.nb-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #1f2329;
}
.nb-types {
  color: #86909c;
  font-size: 10px;
  flex-shrink: 0;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nb-count {
  color: #4e5969;
  font-size: 10px;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
.dp-more {
  margin-top: 3px;
  padding: 2px 6px;
  border: none;
  background: none;
  color: #165dff;
  font-size: 10px;
  cursor: pointer;
}
.dp-call {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 4px;
  border-radius: 4px;
}
.dp-call:hover {
  background: #f2f5fa;
}
.call-loc {
  flex: 1;
  font-family: monospace;
  font-size: 10px;
  color: #1f2329;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.call-type {
  color: #86909c;
  font-size: 10px;
  flex-shrink: 0;
}
.call-count {
  color: #4e5969;
  font-size: 10px;
  flex-shrink: 0;
}
.call-src {
  padding: 1px 5px;
  border: 1px solid #d5dbe3;
  border-radius: 4px;
  background: #fff;
  color: #165dff;
  font-size: 10px;
  cursor: pointer;
  flex-shrink: 0;
}
.dp-foot {
  display: flex;
  gap: 6px;
  padding: 8px 12px;
  border-top: 1px solid #eef1f4;
}
.dp-btn {
  padding: 3px 10px;
  border: 1px solid #d5dae0;
  border-radius: 5px;
  background: #fff;
  color: #4e5969;
  font-size: 11px;
  cursor: pointer;
}
.dp-btn.primary {
  border-color: #165dff;
  background: #165dff;
  color: #fff;
}
.l2-score {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: 10px 16px;
  margin-top: 10px;
  border: 2px solid;
  border-radius: 8px;
  background: #fff;
  flex-wrap: wrap;
}
.score-main {
  display: flex;
  align-items: center;
  gap: 10px;
}
.score-num {
  font-size: 28px;
  font-weight: 700;
}
.score-lv {
  padding: 3px 10px;
  border-radius: 10px;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}
.score-break {
  display: flex;
  gap: 16px;
  font-size: 13px;
  color: #4e5969;
  flex-wrap: wrap;
}
.score-break b {
  color: #1f2329;
}
.score-break b.up {
  color: #165dff;
}
.score-break b.down {
  color: #2f9e6e;
}
.score-break .warn {
  color: #e8684a;
}
.score-formula {
  color: #86909c;
  font-size: 11px;
}
.l2-hint {
  margin: 8px 20px 12px;
  color: #86909c;
  font-size: 12px;
}
.l2-loading {
  padding: 40px;
  text-align: center;
  color: #86909c;
}
</style>
