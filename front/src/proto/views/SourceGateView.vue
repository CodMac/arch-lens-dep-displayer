<script setup lang="ts">
// 首页：选择要分析的源码目录（新建 / 最近打开 / 已有数据源）。
import { ref, computed, onMounted } from 'vue'
import { useSourceStore } from '../../stores/source'
import { fsList } from '../../api/sources'
import type { FsItem } from '../../types'
import { loadRecent, rememberRecent } from '../recentSources'
import type { RecentSource } from '../recentSources'

const emit = defineEmits<{ (e: 'enter'): void }>()

const sourceStore = useSourceStore()

const pathInput = ref('')
const lang = ref('java')
const error = ref('')

/** 最近打开的目录（Cookie 持久化）。 */
const recent = ref<RecentSource[]>([])

/** 最近目录里已就绪的数据源 sid（用于直接进入）。 */
function sidOfRecent(r: RecentSource): string | undefined {
  return sourceStore.sources.find((s) => s.path === r.path && s.status === 'ready')?.sid ?? r.sid
}

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
  void loadFs(item.path)
}

function upFs(): void {
  const p = fsPath.value
  if (!p) return
  const cut = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'))
  void loadFs(cut > 0 ? p.slice(0, cut) : '')
}

function selectDir(): void {
  pathInput.value = fsPath.value
  fsOpen.value = false
}

async function submit(): Promise<void> {
  const p = pathInput.value.trim()
  if (!p) {
    error.value = '请输入源码目录路径'
    return
  }
  error.value = ''
  try {
    const sid = await sourceStore.create(p, lang.value)
    rememberRecent({ path: p, language: lang.value, sid })
    refreshRecent()
    if (sourceStore.activeSid) emit('enter')
  } catch (e) {
    error.value = String(e)
  }
}

function enterSource(sid: string): void {
  void sourceStore.activate(sid).then(() => {
    const s = sourceStore.sources.find((x) => x.sid === sid)
    if (s) {
      rememberRecent({ path: s.path, language: s.language, sid })
      refreshRecent()
    }
    if (sourceStore.activeSid) emit('enter')
  })
}

/** 点击最近目录：已分析过直接进入，否则填入路径待分析。 */
function useRecent(r: RecentSource): void {
  const sid = sidOfRecent(r)
  if (sid) {
    enterSource(sid)
    return
  }
  pathInput.value = r.path
  lang.value = r.language || 'java'
  error.value = ''
}

/** 重新分析中的源 sid（按钮禁用 + 进度展示）。 */
const reanalyzingSid = ref<string | null>(null)

/** 清理旧数据并重新分析；完成后自动激活进入。 */
async function reanalyze(sid: string, path: string, language: string): Promise<void> {
  if (!window.confirm(`将清除「${path}」的旧分析数据并重新分析，确定继续？`)) return
  error.value = ''
  reanalyzingSid.value = sid
  try {
    await sourceStore.reanalyze(sid)
    rememberRecent({ path, language, sid })
    refreshRecent()
    if (sourceStore.activeSid) emit('enter')
  } catch (e) {
    error.value = String(e)
  } finally {
    reanalyzingSid.value = null
  }
}

function refreshRecent(): void {
  recent.value = loadRecent()
}

onMounted(() => {
  refreshRecent()
  void sourceStore.load().then(refreshRecent)
})
</script>

<template>
  <div class="source-gate">
    <header class="sg-head">
      <h2>选择源码目录</h2>
      <p>选择要分析的源码文件夹；已分析过的可直接进入。</p>
    </header>

    <section v-if="recent.length" class="card">
      <h3>最近打开</h3>
      <ul class="recent-list">
        <li v-for="r in recent" :key="r.path" class="recent-item" @click="useRecent(r)">
          <span class="recent-path">{{ r.path }}</span>
          <span class="recent-lang">[{{ r.language }}]</span>
          <span class="recent-act">{{ sidOfRecent(r) ? '进入' : '待分析' }}</span>
          <button
            v-if="sidOfRecent(r)"
            class="btn mini"
            :disabled="reanalyzingSid === sidOfRecent(r)"
            @click.stop="reanalyze(sidOfRecent(r)!, r.path, r.language)"
          >
            {{ reanalyzingSid === sidOfRecent(r) ? '分析中…' : '重新分析' }}
          </button>
        </li>
      </ul>
    </section>

    <section class="card">
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
          <option value="cpp">C++</option>
        </select>
        <button class="btn" @click="openFs">浏览…</button>
        <button class="btn primary" :disabled="sourceStore.creating" @click="submit">
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

    <section class="card">
      <h3>已有数据源（{{ sourceStore.sources.length }}）</h3>
      <div v-if="!sourceStore.sources.length" class="empty">暂无数据源，先创建第一个。</div>
      <ul class="src-list">
        <li
          v-for="s in sourceStore.sources"
          :key="s.sid"
          class="src-item"
          :class="{ active: s.sid === sourceStore.activeSid }"
        >
          <div class="src-info">
            <span class="src-path">{{ s.path }}</span>
            <span class="src-lang">[{{ s.language }}]</span>
            <span class="src-status" :class="s.status">{{ statusText(s.status) }}</span>
            <span v-if="s.sid === sourceStore.activeSid" class="src-active">当前</span>
          </div>
          <div class="src-actions">
            <button v-if="s.status === 'ready'" class="btn" @click="enterSource(s.sid)">进入</button>
            <button
              class="btn"
              :disabled="reanalyzingSid === s.sid"
              @click="reanalyze(s.sid, s.path, s.language)"
            >
              {{ reanalyzingSid === s.sid ? '分析中…' : '重新分析' }}
            </button>
          </div>
        </li>
      </ul>
    </section>

    <div v-if="fsOpen" class="fs-mask" @click="fsOpen = false">
      <div class="fs-panel" @click.stop>
        <div class="fs-head">
          <span class="fs-title">选择源码目录</span>
          <button class="btn" @click="fsOpen = false">×</button>
        </div>
        <div class="fs-bar">
          <button class="btn" :disabled="!fsPath" @click="upFs">⬆ 上级</button>
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
            @click="item.is_dir ? pickDir(item) : undefined"
          >
            <span>{{ item.is_dir ? '📁' : '📄' }}</span>
            <span class="fs-name">{{ item.name }}</span>
          </li>
        </ul>
        <div class="fs-foot">
          <button class="btn primary" @click="selectDir">选择当前目录</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.source-gate {
  max-width: 860px;
  margin: 0 auto;
  padding: 24px 20px;
  height: 100%;
  overflow-y: auto;
}
.sg-head { margin-bottom: 16px; }
.sg-head h2 { font-size: 20px; margin: 0 0 4px; }
.sg-head p { color: #86909c; font-size: 13px; margin: 0; }
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
.btn {
  padding: 8px 14px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
}
.btn.primary { background: #165dff; color: #fff; border-color: #165dff; }
.btn.primary:disabled { opacity: 0.6; cursor: not-allowed; }
.btn.mini { padding: 3px 8px; font-size: 12px; }
.btn:disabled { opacity: 0.6; cursor: not-allowed; }
.src-actions { display: flex; gap: 6px; }
.err { color: #e8684a; font-size: 12px; margin: 8px 0 0; }
.progress { margin-top: 14px; border-top: 1px dashed #e5e6eb; padding-top: 12px; }
.step { display: flex; align-items: center; gap: 8px; padding: 3px 0; font-size: 13px; }
.step-dot { width: 8px; height: 8px; border-radius: 50%; background: #c9cdd4; }
.step-dot.running { background: #165dff; animation: pulse 1s infinite; }
.step-dot.done { background: #5ad8a6; }
.step-dot.failed { background: #e8684a; }
.step-label { font-weight: 600; min-width: 90px; }
.step-msg, .step-dur { color: #86909c; font-size: 12px; }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
.empty { color: #86909c; font-size: 13px; padding: 8px 0; }
.recent-list { list-style: none; margin: 0; padding: 0; }
.recent-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  margin-bottom: 6px;
  cursor: pointer;
  font-size: 13px;
}
.recent-item:hover { border-color: #165dff; background: #f2f7ff; }
.recent-path { flex: 1; font-family: monospace; word-break: break-all; }
.recent-lang { color: #86909c; }
.recent-act { color: #165dff; font-size: 12px; }
.src-list { list-style: none; margin: 0; padding: 0; }
.src-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
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
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.fs-panel {
  width: 560px;
  max-height: 70vh;
  background: #fff;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
}
.fs-head, .fs-bar, .fs-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-bottom: 1px solid #e5e6eb;
}
.fs-head { justify-content: space-between; }
.fs-foot { justify-content: flex-end; border-bottom: none; border-top: 1px solid #e5e6eb; }
.fs-title { font-weight: 600; }
.fs-cur { font-size: 12px; color: #86909c; word-break: break-all; }
.fs-list { list-style: none; margin: 0; padding: 4px 0; overflow-y: auto; flex: 1; }
.fs-item { display: flex; align-items: center; gap: 8px; padding: 6px 16px; cursor: pointer; font-size: 13px; }
.fs-item:hover { background: #f5f7fa; }
.fs-item.dir .fs-name { font-weight: 500; }
.fs-name { word-break: break-all; }
.fs-loading { padding: 24px; text-align: center; color: #86909c; font-size: 13px; }
</style>
