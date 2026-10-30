<script setup lang="ts">
// 源码定位面板（需求11）：右键「查看源码」→ 展示 FilePath:Line，支持复制与编辑器跳转。
import { ref, watch } from 'vue'
import { useDimensionStore } from '../stores/dimension'
import { fetchSource } from '../api/nodes'
import type { SourceInfo } from '../types'

const dimensionStore = useDimensionStore()

const open = ref(false)
const info = ref<SourceInfo | null>(null)
const loading = ref(false)
const error = ref('')
const copied = ref(false)

// 打开面板并加载源码定位。
async function openFor(qn: string): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  open.value = true
  loading.value = true
  error.value = ''
  info.value = null
  try {
    info.value = await fetchSource(dimId, qn)
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

function close(): void {
  open.value = false
}

function copyPath(): void {
  if (!info.value) return
  const text = `${info.value.path}:${info.value.start_line}`
  void navigator.clipboard?.writeText(text).then(() => {
    copied.value = true
    setTimeout(() => { copied.value = false }, 1200)
  })
}

// VSCode 打开协议（vscode://file/absolute:line）。
function openInEditor(): void {
  if (!info.value) return
  const abs = `vscode://file/${info.value.path}:${info.value.start_line}`
  window.open(abs, '_blank')
}

watch(open, (v) => { if (!v) info.value = null })

defineExpose({ openFor })
</script>

<template>
  <transition name="slide">
    <div v-if="open" class="source-panel">
      <header class="sp-head">
        <span>源码定位</span>
        <button class="sp-close" @click="close">×</button>
      </header>
      <div v-if="loading" class="sp-body muted">加载中…</div>
      <div v-else-if="error" class="sp-body error">{{ error }}</div>
      <div v-else-if="info" class="sp-body">
        <div class="sp-row"><span>元素</span><b>{{ info.name }}</b></div>
        <div class="sp-row"><span>文件</span><b class="sp-path">{{ info.path }}</b></div>
        <div class="sp-row"><span>行</span><b>{{ info.start_line }}</b></div>
        <div class="sp-actions">
          <button @click="copyPath">{{ copied ? '已复制' : '复制 File:Line' }}</button>
          <button class="primary" @click="openInEditor">在编辑器中打开</button>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.source-panel {
  position: fixed;
  right: 12px;
  bottom: 12px;
  width: 320px;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  z-index: 40;
  overflow: hidden;
}

.sp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  font-size: 13px;
  font-weight: 600;
  border-bottom: 1px solid #f2f3f5;
}

.sp-close {
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 16px;
  color: #86909c;
}

.sp-body {
  padding: 12px;
  font-size: 12px;
}

.sp-row {
  display: flex;
  gap: 8px;
  line-height: 2;
}

.sp-row span {
  flex: none;
  color: #86909c;
}

.sp-row b {
  color: #1f2329;
  font-weight: 500;
  word-break: break-all;
}

.sp-actions {
  display: flex;
  gap: 8px;
  margin-top: 10px;
}

.sp-actions button {
  flex: 1;
  padding: 5px 0;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 12px;
}

.sp-actions button.primary {
  background: #165dff;
  color: #fff;
  border-color: #165dff;
}

.muted {
  color: #86909c;
}

.error {
  color: #e8684a;
}

.slide-enter-active, .slide-leave-active {
  transition: all 0.2s ease;
}
.slide-enter-from, .slide-leave-to {
  opacity: 0;
  transform: translateY(12px);
}
</style>
