<script setup lang="ts">
// 原型应用外壳：hash 路由 + 面包屑回退 + 视图分发（L1→L4）。
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import L1OverviewView from './views/L1OverviewView.vue'
import L3BlastView from './views/L3BlastView.vue'
import L4SourceView from './views/L4SourceView.vue'
import SourceGateView from './views/SourceGateView.vue'
import { useProtoStore } from './protoStore'
import { decodeRoute, encodeRoute } from './router'
import type { ProtoRoute } from './router'
import { useSourceStore } from '../stores/source'
import { useDimensionStore } from '../stores/dimension'

const store = useProtoStore()
const sourceStore = useSourceStore()
const dimensionStore = useDimensionStore()
/** 维度加载状态；无激活源时显示数据源门禁。 */
const dimLoading = ref(true)
const dimError = ref('')

/** 启动：加载源列表；首页固定为选择源，不自动激活。 */
async function initSources(): Promise<void> {
  dimLoading.value = true
  dimError.value = ''
  try {
    await sourceStore.load()
  } catch (e) {
    dimError.value = String(e)
  } finally {
    dimLoading.value = false
  }
}

/** 加载维度；无激活源时不算错误（由选择源页面引导）。 */
async function loadDimensions(): Promise<void> {
  if (!sourceStore.activeSid) return
  await dimensionStore.load()
  if (dimensionStore.currentId == null) dimError.value = '后端无可用维度：请先创建并激活数据源'
}

/** 选择源页面：创建/激活/重新分析后进入全景。 */
async function onEnter(): Promise<void> {
  dimLoading.value = true
  try {
    // 维度 id 随数据源（甚至重新分析）变化：每次从选择页进入都清空重载。
    dimensionStore.reset()
    store.reset()
    await loadDimensions()
    if (dimensionStore.currentId != null) {
      dimError.value = ''
      goOverview()
    }
  } finally {
    dimLoading.value = false
  }
}

/** 返回选择源页面（L1 分析页可随时调回）。 */
function goSources(): void {
  store.reset()
  applyRoute({ name: 'sources' })
}
// 面包屑数组（模板用，composable 返回 ref 需在此解包）。
const crumbs = store.crumbs

const route = ref<ProtoRoute>({ name: 'overview' })

// hash 应用/同步防环。
let applyingHash = false
// 自同步标记：syncHash 写入 hash 触发的 hashchange 由自身产生，route 已含完整
// from/targetQn（hash 编码是有损子集），应跳过 decode 重放以免丢失入口信息。
let selfSync = false

function applyRoute(r: ProtoRoute): void {
  route.value = r
}

function syncHash(): void {
  const h = encodeRoute(route.value)
  if (window.location.hash !== h) {
    selfSync = true
    window.location.hash = h
  }
}

function onHashChange(): void {
  if (applyingHash) return
  if (selfSync) {
    selfSync = false // 自己写入的 hash：route 已应用，跳过重放
    return
  }
  applyRoute(decodeRoute(window.location.hash))
}

// 视图跳转：L1→L2→L3（L1/L2 合并后无独立 L2 页；术语统一：L2=影响面/蝴蝶、L3=源码）。
function goBlast(qn: string, method?: string): void {
  store.enter({ label: 'L2 影响面', route: 'l2', qn, method })
  applyRoute({ name: 'l2', qn, method })
}
/** 类/包模式下点击节点 → 以该节点为新中心（同层替换，不压栈）。 */
function recenterBlast(qn: string): void {
  applyRoute({ name: 'l2', qn })
}
function goSource(qn: string): void {
  store.enter({ label: 'L3 源码', route: 'l3', qn })
  applyRoute({ name: 'l3', qn })
}
/** 11.20：L2 调用点 → L3 源码（带行号 + 两端方法，精确高亮调用点）。 */
function goSourceLine(srcQn: string, dstQn: string, line: number): void {
  store.enter({ label: 'L3 源码', route: 'l3', qn: srcQn })
  applyRoute({ name: 'l3', qn: srcQn, qn2: dstQn, line, srcQn, dstQn })
}
function goOverview(): void {
  store.reset()
  applyRoute({ name: 'overview' })
}

/** 顶部品牌按钮：有激活源进全景，否则回选择源页。 */
function onBrand(): void {
  if (sourceStore.activeSid) goOverview()
  else goSources()
}

// 面包屑回退。
function onCrumb(index: number): void {
  const c = store.crumbs.value[index]
  if (!c) return
  store.backTo(index)
  if (c.route === 'overview') applyRoute({ name: 'overview' })
  else if (c.route === 'l2' && c.qn) applyRoute({ name: 'l2', qn: c.qn, method: c.method })
  else if (c.route === 'l3' && c.qn) applyRoute({ name: 'l3', qn: c.qn })
}

// L3/L4 返回：弹出当前层，回到上一层（breadcrumb 顶部）。
function goBackOne(): void {
  const len = store.crumbs.value.length
  if (len <= 1) {
    goOverview()
    return
  }
  onCrumb(len - 2)
}

// 路由变化 → 同步 hash（防抖）。
let syncTimer: number | undefined
watch(
  () => route.value,
  () => {
    window.clearTimeout(syncTimer)
    syncTimer = window.setTimeout(syncHash, 60)
  },
  { deep: true },
)

onMounted(() => {
  window.addEventListener('hashchange', onHashChange)
  // 初始应用 hash 或默认全景。
  applyingHash = true
  try {
    applyRoute(decodeRoute(window.location.hash))
  } finally {
    applyingHash = false
  }
  syncHash()
  void initSources()
})

onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHashChange)
  window.clearTimeout(syncTimer)
})
</script>

<template>
  <div class="proto-app">
    <header class="proto-bar">
      <button class="proto-brand" @click="onBrand">◆ Arch-Lens</button>
      <!-- 面包屑（5.1）；选择源页面不显示 -->
      <nav v-if="route.name !== 'sources'" class="crumbs">
        <template v-for="(c, i) in crumbs" :key="i">
          <span v-if="i > 0" class="crumb-sep">/</span>
          <button class="crumb" :class="{ active: i === crumbs.length - 1 }" @click="onCrumb(i)">
            {{ c.label }}
          </button>
        </template>
      </nav>
      <span v-else class="crumbs" />
      <button v-if="route.name !== 'sources'" class="proto-switch" @click="goSources">切换数据源</button>
      <span class="proto-tag">真实数据 · 后端接口</span>
    </header>
    <div class="proto-body">
      <div v-if="dimLoading" class="proto-gate">加载中…</div>
      <SourceGateView v-else-if="route.name === 'sources' || !sourceStore.activeSid" @enter="onEnter" />
      <div v-else-if="dimError" class="proto-gate proto-gate-err">{{ dimError }}</div>
      <L1OverviewView
        v-else-if="route.name === 'overview'"
        @go-source="goSource"
        @go-blast="goBlast"
        @go-blast-method="goBlast"
      />
      <L3BlastView
        v-else-if="route.name === 'l2'"
        :qn="route.qn"
        :method="route.method"
        @go-source="goSource"
        @go-source-line="goSourceLine"
        @recenter="recenterBlast"
        @back="goBackOne"
      />
      <L4SourceView
        v-else-if="route.name === 'l3'"
        :qn="route.qn"
        :qn2="route.qn2"
        :highlight-line="route.line"
        :src-qn="route.srcQn"
        :dst-qn="route.dstQn"
        @back="goBackOne"
      />
    </div>
  </div>
</template>

<style scoped>
.proto-app {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.proto-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 16px;
  height: 44px;
  background: #1f2329;
  color: #fff;
  z-index: 30;
}
.proto-brand {
  border: none;
  background: transparent;
  color: #fff;
  font-weight: 600;
  font-size: 14px;
  cursor: pointer;
}
.proto-brand:hover {
  color: #5B8FF9;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
}
.crumb {
  border: none;
  background: transparent;
  color: #c0c4cc;
  font-size: 13px;
  cursor: pointer;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.crumb:hover {
  color: #fff;
}
.crumb.active {
  color: #5B8FF9;
  font-weight: 600;
}
.crumb-sep {
  color: #4e5969;
}
.proto-switch {
  border: 1px solid #4e5969;
  background: transparent;
  color: #c0c4cc;
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
}
.proto-switch:hover {
  color: #fff;
  border-color: #5B8FF9;
}
.proto-tag {
  font-size: 12px;
  color: #86909c;
}
.proto-body {
  flex: 1;
  min-height: 0;
  position: relative;
}
.proto-gate {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #86909c;
  font-size: 14px;
}
.proto-gate-err {
  color: #e8684a;
}
</style>
