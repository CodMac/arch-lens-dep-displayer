<script setup lang="ts">
// 应用根：数据源首页（home）↔ L0 全景（overview）↔ L1+ 层级图（level）。
// 多数据源改造：无激活数据源时停留在 home 引导创建；激活后进入全景。
// D6 需求20：下钻路径同步到 location.hash（以数据源 sid 定位），刷新/分享不丢。
import { ref, watch, computed, onMounted, onBeforeUnmount } from 'vue'
import SourceHomeView from './views/SourceHomeView.vue'
import OverviewView from './views/OverviewView.vue'
import LevelGraphView from './views/LevelGraphView.vue'
import { useDimensionStore } from './stores/dimension'
import { useLayerStackStore } from './stores/layerStack'
import { useSourceStore } from './stores/source'
import { encode as encodeHash, decode as decodeHash } from './utils/hashState'

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()
const sourceStore = useSourceStore()

// 视图模式：home（数据源首页）| overview（首页全景）| level（层级图）。
const mode = ref<'home' | 'overview' | 'level'>('home')

// 无激活源时始终显示 home（数据源首页）。
const effectiveMode = computed(() => (sourceStore.activeSid ? mode.value : 'home'))

// hash 应用/同步防环。
let applyingHash = false
let dimsLoaded = false

function showLevel(): void {
  mode.value = 'level'
}

function showOverview(): void {
  layerStack.reset()
  mode.value = 'overview'
}

// 从 home 进入（创建或激活成功后）。
function onEnter(): void {
  if (!sourceStore.activeSid) return
  void dimensionStore.load().then(() => {
    mode.value = 'overview'
  })
}

// 状态 → hash（防抖）。无激活源或 home 模式下不写。
let syncTimer: number | undefined
function syncHashFromState(): void {
  if (applyingHash || !dimsLoaded || !sourceStore.activeSid) return
  window.clearTimeout(syncTimer)
  syncTimer = window.setTimeout(() => {
    const h = encodeHash({
      sid: sourceStore.activeSid!,
      mode: mode.value === 'level' ? 'level' : 'overview',
      stack: layerStack.stack.filter((s) => s.level > 0).map((s) => ({ level: s.level, qn: s.qn })),
    })
    if (window.location.hash !== h) {
      window.location.hash = h
    }
  }, 60)
}

// hash → 状态。
function applyHashToState(hash: string): void {
  const st = decodeHash(hash)
  if (!st) return
  applyingHash = true
  try {
    void sourceStore.activate(st.sid)
    mode.value = st.mode
    layerStack.hydrate(st.stack)
  } finally {
    applyingHash = false
  }
}

function onHashChange(): void {
  if (applyingHash) return
  const st = decodeHash(window.location.hash)
  if (st) {
    const same =
      st.sid === sourceStore.activeSid &&
      st.mode === mode.value &&
      st.stack.map((s) => `${s.level}:${s.qn}`).join('/') ===
        layerStack.stack.filter((s) => s.level > 0).map((s) => `${s.level}:${s.qn}`).join('/')
    if (!same) applyHashToState(window.location.hash)
  }
}

// 激活源变化 → 加载维度（维度接口随激活源变化）。
watch(
  () => sourceStore.activeSid,
  () => {
    layerStack.reset()
    dimensionStore.reset()
    if (sourceStore.activeSid) {
      void dimensionStore.load()
    }
  },
)

// 视图/栈/激活源变化 → 同步 hash。
watch(
  () => [mode.value, layerStack.stack.map((s) => s.qn).join('>'), sourceStore.activeSid],
  () => syncHashFromState(),
)

onMounted(() => {
  window.addEventListener('hashchange', onHashChange)
  void sourceStore.load().then(() => {
    dimsLoaded = true
    // 有激活源：应用 hash 或写默认；否则留在 home。
    if (sourceStore.activeSid) {
      const h = window.location.hash
      if (h && h.startsWith('#/d/')) {
        applyHashToState(h)
      } else {
        mode.value = 'overview'
        syncHashFromState()
      }
    } else {
      mode.value = 'home'
    }
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHashChange)
  window.clearTimeout(syncTimer)
})
</script>

<template>
  <div class="app-root">
    <header class="app-bar">
      <button
        v-if="sourceStore.activeSid"
        class="home-btn"
        :class="{ active: effectiveMode === 'overview' }"
        @click="showOverview"
      >
        ◉ 仓库全景
      </button>
      <span v-if="sourceStore.activeSid" class="src-tag">{{ sourceStore.activeSource?.path }}</span>
      <span class="app-title">arch-lens 依赖图</span>
    </header>
    <div class="app-body">
      <SourceHomeView v-if="effectiveMode === 'home'" @enter="onEnter" />
      <OverviewView v-else-if="effectiveMode === 'overview'" @enter="showLevel" />
      <LevelGraphView v-else />
    </div>
  </div>
</template>

<style scoped>
.app-root {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.app-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  height: 44px;
  background: #fff;
  border-bottom: 1px solid #e5e6eb;
  z-index: 30;
}

.home-btn {
  padding: 4px 12px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
  color: #4e5969;
}

.home-btn.active {
  background: #e8f3ff;
  border-color: #165dff;
  color: #165dff;
}

.app-title {
  font-size: 14px;
  font-weight: 600;
  color: #1f2329;
}

.src-tag {
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 2px 8px;
  background: #f5f7fa;
  border-radius: 4px;
  color: #86909c;
  font-size: 12px;
  font-family: monospace;
}

.app-body {
  flex: 1;
  min-height: 0;
  position: relative;
}
</style>
