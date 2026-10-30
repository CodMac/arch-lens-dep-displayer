<script setup lang="ts">
// 数据信息侧栏（D4-2/3/4）：数据版本/统计信息、异常数据可见（孤立/游离）、指标 TOP 排序列表。
// 需求15 数据可信度（导入时间/统计）｜需求16 异常数据可见｜需求17 指标排序 TOP。
import { ref, watch, computed } from 'vue'
import { useDimensionStore } from '../stores/dimension'
import { useLayerStackStore } from '../stores/layerStack'
import { fetchMeta, fetchTopBy, fetchScc } from '../api/metrics'
import type { MetaInfo, TopResult } from '../types'

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()

const open = ref(false)
const meta = ref<MetaInfo | null>(null)
const topTab = ref<'fanout' | 'fanin' | 'size'>('fanout')
const topData = ref<TopResult | null>(null)
const topLoading = ref(false)
const cycleCount = ref(0)

// 统计展示行（key → 中文标签）。
const STAT_LABELS: Record<string, string> = {
  element_total: '元素总数',
  class_total: '类总数',
  package_total: '包总数',
  relation_total: '关系总数',
}

const statRows = computed(() => {
  const s = meta.value?.stats ?? {}
  return Object.entries(STAT_LABELS)
    .filter(([k]) => k in s)
    .map(([k, label]) => ({ label, value: s[k] ?? 0 }))
})

const topTabs = [
  { key: 'fanout', label: '扇出 TOP' },
  { key: 'fanin', label: '扇入 TOP' },
  { key: 'size', label: '规模 TOP' },
] as const

async function loadMeta(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  try {
    meta.value = await fetchMeta(dimId)
    const comps = await fetchScc(dimId)
    cycleCount.value = comps.length
  } catch {
    meta.value = null
  }
}

async function loadTop(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId || !open.value) return
  topLoading.value = true
  try {
    topData.value = await fetchTopBy(dimId, topTab.value, 10)
  } catch {
    topData.value = null
  } finally {
    topLoading.value = false
  }
}

function toggle(): void {
  open.value = !open.value
  if (open.value) {
    void loadMeta()
    void loadTop()
  }
}

function switchTab(t: 'fanout' | 'fanin' | 'size'): void {
  topTab.value = t
  void loadTop()
}

// 点击 TOP/异常项：定位到节点（尽量下钻）。
function locate(qn: string, level = 2): void {
  layerStack.reset()
  layerStack.push({ level, qn, label: qn.split('.').pop() ?? qn, kind: '' })
}

function fmtTime(t?: string): string {
  if (!t) return '—'
  return t.replace('T', ' ').slice(0, 19)
}

// 切维度重载。
watch(() => dimensionStore.currentId, () => {
  if (open.value) {
    void loadMeta()
    void loadTop()
  }
})
</script>

<template>
  <div class="info-panel" :class="{ open }">
    <button class="toggle" title="数据信息与指标" @click="toggle">
      ☰
    </button>

    <transition name="slide">
      <div v-if="open" class="panel-body">
        <!-- 需求15 数据版本/统计 -->
        <section class="sec">
          <h4>数据信息</h4>
          <div class="kv">
            <span>导入时间</span><b>{{ fmtTime(meta?.imported_at) }}</b>
          </div>
          <div v-for="r in statRows" :key="r.label" class="kv">
            <span>{{ r.label }}</span><b>{{ r.value }}</b>
          </div>
          <div class="kv">
            <span>关系类型</span><b>{{ meta?.relation_types ?? '—' }}</b>
          </div>
          <div class="kv">
            <span>环数</span><b>{{ cycleCount }}</b>
          </div>
        </section>

        <!-- 需求16 异常数据可见 -->
        <section class="sec">
          <h4>异常数据</h4>
          <div class="kv">
            <span>孤立节点</span>
            <b :class="{ warn: (meta?.isolated_nodes ?? 0) > 0 }">{{ meta?.isolated_nodes ?? '—' }}</b>
          </div>
          <div v-if="(meta?.isolated_list?.length ?? 0)" class="mini-list">
            <div v-for="qn in meta?.isolated_list" :key="qn" class="mini-item" @click="locate(qn)">
              {{ qn }}
            </div>
          </div>
          <div class="kv">
            <span>游离元素</span>
            <b :class="{ warn: (meta?.unknown_kinds ?? 0) > 0 }">{{ meta?.unknown_kinds ?? '—' }}</b>
          </div>
          <div v-if="(meta?.unknown_list?.length ?? 0)" class="mini-list">
            <div v-for="u in meta?.unknown_list?.slice(0, 15)" :key="u.qn" class="mini-item" @click="locate(u.qn)">
              <span class="uk" :title="u.kind">{{ u.kind }}</span>{{ u.qn }}
            </div>
          </div>
        </section>

        <!-- 需求17 指标排序 TOP -->
        <section class="sec">
          <h4>指标排序</h4>
          <div class="tabs">
            <button
              v-for="t in topTabs"
              :key="t.key"
              class="tab"
              :class="{ active: topTab === t.key }"
              @click="switchTab(t.key)"
            >{{ t.label }}</button>
          </div>
          <div v-if="topLoading" class="muted">加载中…</div>
          <ol v-else class="top-list">
            <li v-for="(it, i) in topData?.items ?? []" :key="it.qn" class="top-item" @click="locate(it.qn)">
              <span class="rank" :class="{ hot: i < 3 }">{{ i + 1 }}</span>
              <span class="t-name">{{ it.label }}</span>
              <span class="t-val">{{ it.value }}</span>
            </li>
          </ol>
        </section>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.info-panel {
  position: fixed;
  right: 12px;
  top: 60px;
  z-index: 40;
}

.toggle {
  width: 34px;
  height: 34px;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
  font-size: 16px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.08);
}

.panel-body {
  position: absolute;
  right: 0;
  top: 40px;
  width: 300px;
  max-height: calc(100vh - 120px);
  overflow-y: auto;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  padding: 12px;
}

.slide-enter-active, .slide-leave-active {
  transition: all 0.2s ease;
}
.slide-enter-from, .slide-leave-to {
  opacity: 0;
  transform: translateX(12px);
}

.sec {
  margin-bottom: 14px;
}

.sec h4 {
  margin: 0 0 8px;
  font-size: 13px;
  color: #1f2329;
  border-bottom: 1px solid #f2f3f5;
  padding-bottom: 6px;
}

.kv {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  line-height: 2;
  color: #86909c;
}

.kv b {
  color: #1f2329;
  font-weight: 500;
}

.kv b.warn {
  color: #e8684a;
}

.mini-list {
  margin: 4px 0 8px;
  max-height: 140px;
  overflow-y: auto;
}

.mini-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  font-size: 11px;
  color: #4e5969;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mini-item:hover {
  background: #f2f3f5;
  color: #165dff;
}

.uk {
  flex: none;
  padding: 0 4px;
  border-radius: 3px;
  background: #fff1e8;
  color: #d25f00;
  font-size: 10px;
}

.tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 8px;
}

.tab {
  flex: 1;
  padding: 4px 0;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 12px;
  color: #4e5969;
}

.tab.active {
  background: #165dff;
  border-color: #165dff;
  color: #fff;
}

.top-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.top-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  font-size: 12px;
  cursor: pointer;
  border-radius: 4px;
}

.top-item:hover {
  background: #f2f3f5;
}

.rank {
  flex: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #f2f3f5;
  color: #86909c;
  font-size: 11px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.rank.hot {
  background: #165dff;
  color: #fff;
}

.t-name {
  flex: 1;
  color: #1f2329;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.t-val {
  color: #e8684a;
  font-weight: 600;
}

.muted {
  color: #86909c;
  font-size: 12px;
}
</style>
