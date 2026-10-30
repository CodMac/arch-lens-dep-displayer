<script setup lang="ts">
// 数据源首页（多数据源改造）：启动后的默认视图。
// 提供源码路径输入 + 目录浏览弹层 + 已有数据源列表（激活/切换）。
// 无激活源时引导创建并分析；创建后轮询进度（source store）。
import { ref, computed, onMounted } from 'vue'
import { useSourceStore } from '../stores/source'
import { fsList } from '../api/sources'
import type { FsItem } from '../types'

const emit = defineEmits<{ (e: 'enter'): void }>()

const sourceStore = useSourceStore()

const pathInput = ref('')
const lang = ref('java')
const error = ref('')

// 目录浏览弹层。
const fsOpen = ref(false)
const fsPath = ref('')
const fsItems = ref<FsItem[]>([])
const fsLoading = ref(false)
const fsError = ref('')

const analyzingSource = computed(() =>
  sourceStore.sources.find((x) => x.status === 'analyzing'),
)

function statusText(s: string): string {
  switch (s) {
    case 'ready': return '已就绪'
    case 'analyzing': return '分析中'
    case 'failed': return '失败'
    default: return '未分析'
  }
}

async function openFs(): Promise<void> {
  fsOpen.value = true
  fsPath.value = ''
  await loadFs('')
}

async function loadFs(path?: string): Promise<void> {
  fsLoading.value = true
  fsError.value = ''
  try {
    const res = await fsList(path)
    fsPath.value = res.path
    fsItems.value = res.items
  } catch (e) {
    fsError.value = String(e)
  } finally {
    fsLoading.value = false
  }
}

function pickDir(item: FsItem): void {
  if (!item.is_dir) return
  loadFs(item.path)
}

function upFs(): void {
  const p = fsPath.value
  if (!p) return
  const idx = p.lastIndexOf('')
  const idx2 = p.lastIndexOf('/')
  const cut = Math.max(idx, idx2)
  const parent = cut > 0 ? p.slice(0, cut) : ''
  void loadFs(parent)
}

function selectDir(): void {
  pathInput.value = fsPath.value
  fsOpen.value = false
}

function selectItem(item: FsItem): void {
  if (item.is_dir) pathInput.value = item.path
}

async function submit(): Promise<void> {
  const p = pathInput.value.trim()
  if (!p) {
    error.value = '请输入源码目录路径'
    return
  }
  error.value = ''
  try {
    await sourceStore.create(p, lang.value)
    if (sourceStore.activeSid) emit('enter')
  } catch (e) {
    error.value = String(e)
  }
}

function enterSource(sid: string): void {
  void sourceStore.activate(sid).then(() => {
    if (sourceStore.activeSid) emit('enter')
  })
}

onMounted(() => {
  void sourceStore.load()
})
</script>
<template>
  <div class="source-home">
    <header class="sh-head">
      <h2 class="heading">数据源管理</h2>
      <p class="sub">选择要分析的源码目录；已分析过的可直接进入。</p>
    </header>

    <section class="sh-create card">
      <h3>新建数据源</h3>
      <div class="create-row">
        <input
          v-model="pathInput"
          class="path-input"
          placeholder="输入源码目录绝对路径，或浏览选择"
          @keyup.enter="submit"
        />
        <select v-model="lang" class="lang-select">
          <option value="java">Java</option>
        </select>
        <button class="browse-btn" @click="openFs">浏览…</button>
        <button class="primary" :disabled="sourceStore.creating" @click="submit">
          {{ sourceStore.creating ? '分析中…' : '开始分析' }}
        </button>
      </div>
      <p v-if="error" class="err">{{ error }}</p>
      <p v-if="sourceStore.error" class="err">{{ sourceStore.error }}</p>

      <div v-if="analyzingSource" class="progress">
        <div v-for="s in analyzingSource.steps ?? []" :key="s.id" class="step">
          <span class="step-dot" :class="s.status"></span>
          <span class="step-label">{{ s.label }}</span>
          <span class="step-msg">{{ s.message }}</span>
          <span v-if="s.duration > 0" class="step-dur">{{ s.duration.toFixed(1) }}s</span>
        </div>
      </div>
    </section>

    <section class="sh-list card">
      <h3>已有数据源（{{ sourceStore.sources.length }}）</h3>
      <div v-if="!sourceStore.sources.length" class="empty">暂无数据源，先创建第一个。</div>
      <ul class="src-list">
        <li v-for="s in sourceStore.sources" :key="s.sid" class="src-item" :class="{ active: s.sid === sourceStore.activeSid }">
          <div class="src-info">
            <span class="src-path">{{ s.path }}</span>
            <span class="src-lang">[{{ s.language }}]</span>
            <span class="src-status" :class="s.status">{{ statusText(s.status) }}</span>
            <span v-if="s.sid === sourceStore.activeSid" class="src-active">当前</span>
          </div>
          <div class="src-actions">
            <button v-if="s.status === 'ready'" class="enter-btn" @click="enterSource(s.sid)">进入</button>
          </div>
        </li>
      </ul>
    </section>

    <div v-if="fsOpen" class="fs-mask" @click="fsOpen = false">
      <div class="fs-panel" @click.stop>
        <div class="fs-head">
          <span class="fs-title">选择源码目录</span>
          <button class="fs-close" @click="fsOpen = false">×</button>
        </div>
        <div class="fs-bar">
          <button class="up-btn" :disabled="!fsPath" @click="upFs">⬆ 上级</button>
          <span class="fs-cur">{{ fsPath || '（根）' }}</span>
        </div>
        <div v-if="fsLoading" class="fs-loading">加载中…</div>
        <div v-else-if="fsError" class="fs-loading err">{{ fsError }}</div>
        <ul v-else class="fs-list">
          <li
            v-for="item in fsItems"
            :key="item.path"
            class="fs-item"
            :class="{ dir: item.is_dir }"
            @click="item.is_dir ? pickDir(item) : selectItem(item)"
          >
            <span class="fs-icon">{{ item.is_dir ? '📁' : '📄' }}</span>
            <span class="fs-name">{{ item.name }}</span>
          </li>
        </ul>
        <div class="fs-foot">
          <button class="primary" @click="selectDir">选择当前目录</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.source-home {
  max-width: 860px;
  margin: 0 auto;
  padding: 24px 20px;
  height: 100%;
  overflow-y: auto;
}

.sh-head { margin-bottom: 16px; }
.heading { font-size: 20px; margin: 0 0 4px; }
.sub { color: #86909c; font-size: 13px; margin: 0; }

.card {
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}
.card h3 { margin: 0 0 12px; font-size: 15px; }

.create-row { display: flex; gap: 8px; }
.path-input {
  flex: 1;
  padding: 8px 10px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  font-size: 13px;
}
.lang-select { padding: 8px; border: 1px solid #e5e6eb; border-radius: 6px; }
.browse-btn, .enter-btn, .up-btn, .fs-close {
  padding: 8px 14px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
}
button.primary {
  background: #165dff;
  color: #fff;
  border: 1px solid #165dff;
  border-radius: 6px;
  padding: 8px 16px;
  cursor: pointer;
  font-size: 13px;
}
button.primary:disabled { opacity: 0.6; cursor: not-allowed; }
.err { color: #e8684a; font-size: 12px; margin: 8px 0 0; }

.progress { margin-top: 14px; border-top: 1px dashed #e5e6eb; padding-top: 12px; }
.step { display: flex; align-items: center; gap: 8px; padding: 3px 0; font-size: 13px; }
.step-dot { width: 8px; height: 8px; border-radius: 50%; }
.step-dot.pending { background: #c9cdd4; }
.step-dot.running { background: #165dff; animation: pulse 1s infinite; }
.step-dot.done { background: #5ad8a6; }
.step-dot.failed { background: #e8684a; }
.step-dot.skipped { background: #c9cdd4; }
.step-label { font-weight: 600; min-width: 90px; }
.step-msg { color: #86909c; font-size: 12px; }
.step-dur { color: #86909c; font-size: 12px; }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }

.empty { color: #86909c; font-size: 13px; padding: 8px 0; }
.src-list { list-style: none; margin: 0; padding: 0; }
.src-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 12px; border: 1px solid #e5e6eb; border-radius: 6px;
  margin-bottom: 8px;
}
.src-item.active { border-color: #165dff; background: #f2f7ff; }
.src-info { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.src-path { font-family: monospace; }
.src-lang { color: #86909c; }
.src-status { padding: 1px 8px; border-radius: 10px; font-size: 12px; }
.src-status.ready { background: #e8faf0; color: #00875a; }
.src-status.analyzing { background: #e8f3ff; color: #165dff; }
.src-status.failed { background: #ffece8; color: #e8684a; }
.src-active { color: #165dff; font-weight: 600; }

.fs-mask {
  position: fixed; inset: 0; background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.fs-panel {
  width: 560px; max-height: 70vh; background: #fff; border-radius: 8px;
  display: flex; flex-direction: column;
}
.fs-head { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid #e5e6eb; }
.fs-title { font-weight: 600; }
.fs-bar { display: flex; align-items: center; gap: 8px; padding: 8px 16px; border-bottom: 1px solid #e5e6eb; }
.fs-cur { font-size: 12px; color: #86909c; word-break: break-all; }
.fs-list { list-style: none; margin: 0; padding: 4px 0; overflow-y: auto; flex: 1; }
.fs-item { display: flex; align-items: center; gap: 8px; padding: 6px 16px; cursor: pointer; font-size: 13px; }
.fs-item:hover { background: #f5f7fa; }
.fs-item.dir .fs-name { font-weight: 500; }
.fs-name { word-break: break-all; }
.fs-loading { padding: 24px; text-align: center; color: #86909c; font-size: 13px; }
.fs-foot { display: flex; justify-content: flex-end; padding: 12px 16px; border-top: 1px solid #e5e6eb; }
</style>
