<script setup lang="ts">
// 收藏侧栏（D6 需求14）：列出收藏节点，支持定位 / 取消收藏 / 编辑备注。
import { ref, watch } from 'vue'
import { useDimensionStore } from '../stores/dimension'
import { useLayerStackStore } from '../stores/layerStack'
import { useAnnotationsStore } from '../stores/annotations'
import type { Annotation } from '../types'

const dimensionStore = useDimensionStore()
const layerStack = useLayerStackStore()
const annotations = useAnnotationsStore()

const open = ref(false)
// 正在编辑备注的 QN（展开 inline 编辑框）。
const editingQN = ref('')
const noteDraft = ref('')

function toggle(): void {
  open.value = !open.value
  if (open.value) void reload()
}

async function reload(): Promise<void> {
  const dimId = dimensionStore.currentId
  if (dimId) await annotations.load(dimId)
}

// 定位到节点（复用 DataInfoPanel.locate 模式）。
function locate(a: Annotation): void {
  layerStack.reset()
  layerStack.push({ level: a.kind ? levelOf(a.kind) : 2, qn: a.qn, label: a.label ?? a.qn.split('.').pop() ?? a.qn, kind: a.kind ?? '' })
}

// 由 kind 粗略推导层级（PACKAGE=1，其余默认类层 2）。
function levelOf(kind: string): number {
  return kind === 'PACKAGE' ? 1 : 2
}

async function remove(a: Annotation): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  await annotations.toggleStar(dimId, a.qn)
}

function startEdit(a: Annotation): void {
  editingQN.value = a.qn
  noteDraft.value = a.note ?? ''
}

async function saveNote(a: Annotation): Promise<void> {
  const dimId = dimensionStore.currentId
  if (!dimId) return
  await annotations.setNote(dimId, a.qn, noteDraft.value)
  editingQN.value = ''
}

function fmtTime(t?: string): string {
  if (!t) return '—'
  return t.replace('T', ' ').slice(0, 16)
}

// 切维度重载。
watch(() => dimensionStore.currentId, () => {
  editingQN.value = ''
  if (open.value) void reload()
})
</script>

<template>
  <div class="star-panel" :class="{ open }">
    <button class="toggle" title="节点收藏" @click="toggle">★</button>

    <transition name="slide">
      <div v-if="open" class="panel-body">
        <h4>收藏节点</h4>
        <div v-if="annotations.loading" class="muted">加载中…</div>
        <div v-else-if="!annotations.starredList.length" class="muted empty">
          暂无收藏。右键节点 →「★ 收藏」加入列表。
        </div>
        <ul v-else class="star-list">
          <li v-for="a in annotations.starredList" :key="a.qn" class="star-item">
            <div class="s-head" @click="locate(a)">
              <span class="s-star">★</span>
              <span class="s-name">{{ a.label }}</span>
              <span v-if="a.kind" class="s-kind">{{ a.kind }}</span>
            </div>
            <div class="s-note">
              <template v-if="editingQN === a.qn">
                <textarea v-model="noteDraft" rows="2" placeholder="备注…" />
                <div class="s-actions">
                  <button @click="saveNote(a)">保存</button>
                  <button @click="editingQN = ''">取消</button>
                </div>
              </template>
              <template v-else>
                <span class="s-note-text" :class="{ none: !a.note }" @click="startEdit(a)">
                  {{ a.note || '无备注，点击添加' }}
                </span>
                <div class="s-actions">
                  <button title="编辑备注" @click="startEdit(a)">✎</button>
                  <button title="取消收藏" class="danger" @click="remove(a)">✕</button>
                </div>
              </template>
            </div>
            <div class="s-meta">
              <span class="s-qn" :title="a.qn">{{ a.qn }}</span>
              <span class="s-time">{{ fmtTime(a.updated_at) }}</span>
            </div>
          </li>
        </ul>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.star-panel {
  position: fixed;
  left: 12px;
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
  color: #d4a017;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.08);
}

.panel-body {
  position: absolute;
  left: 0;
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

.panel-body h4 {
  margin: 0 0 8px;
  font-size: 13px;
  color: #1f2329;
  border-bottom: 1px solid #f2f3f5;
  padding-bottom: 6px;
}

.slide-enter-active, .slide-leave-active {
  transition: all 0.2s ease;
}
.slide-enter-from, .slide-leave-to {
  opacity: 0;
  transform: translateX(-12px);
}

.muted {
  color: #86909c;
  font-size: 12px;
}

.muted.empty {
  line-height: 1.8;
}

.star-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.star-item {
  padding: 8px;
  border: 1px solid #f2f3f5;
  border-radius: 6px;
  margin-bottom: 8px;
}

.s-head {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}

.s-star {
  color: #d4a017;
  font-size: 13px;
}

.s-name {
  font-size: 13px;
  font-weight: 600;
  color: #1f2329;
}

.s-kind {
  flex: none;
  padding: 0 4px;
  border-radius: 3px;
  background: #e8f3ff;
  color: #165dff;
  font-size: 10px;
}

.s-note {
  margin-top: 6px;
}

.s-note-text {
  display: block;
  font-size: 12px;
  color: #4e5969;
  cursor: pointer;
  line-height: 1.6;
  word-break: break-all;
}

.s-note-text.none {
  color: #c9cdd4;
}

.s-note textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 4px 6px;
  border: 1px solid #e5e6eb;
  border-radius: 4px;
  font-size: 12px;
  resize: vertical;
}

.s-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 4px;
}

.s-actions button {
  padding: 2px 8px;
  border: 1px solid #e5e6eb;
  border-radius: 4px;
  background: #fff;
  cursor: pointer;
  font-size: 11px;
  color: #4e5969;
}

.s-actions button:hover {
  color: #165dff;
  border-color: #165dff;
}

.s-actions button.danger:hover {
  color: #e8684a;
  border-color: #e8684a;
}

.s-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-top: 6px;
  font-size: 10px;
  color: #86909c;
}

.s-qn {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
