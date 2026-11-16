<script setup lang="ts">
// 单节点源码面板（11.14 从 L4SourceView 抽出，支持 L3 双节点并排预览）。
// 左侧元素层级树 + 右侧源码全文件；点左树 → 右源码高亮行范围；点源码行 → 左树选中元素。
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useProtoStore } from '../protoStore'
import type { SourceElement } from '../types/model'

const props = defineProps<{
  qn: string
  /** 11.15：初始高亮行（实际边跳转时定位到调用点）。 */
  highlightLine?: number
}>()
const store = useProtoStore()

const file = ref<{ path: string; content: string; lineCount: number; elements: SourceElement[] } | null>(null)
const sourceRef = ref<HTMLPreElement>()

// 可调宽分栏。
const treeWidth = ref(220)
const dragging = ref(false)

// 高亮行范围（双向）。
const highlight = ref<{ start: number; end: number } | null>(null)
const selectedQn = ref<string | null>(null)

const kindColor: Record<string, string> = {
  CLASS: '#5B8FF9', METHOD: '#5AD8A6', FIELD: '#6DC8EC',
  CONSTRUCTOR: '#9270CA', LAMBDA: '#FF9D4D', METHOD_REF: '#269A99',
  SCOPE_BLOCK: '#A0A0A0', INNER_CLASS: '#F6BD16', ANONYMOUS_CLASS: '#9270CA',
}

const lines = computed(() => file.value?.content.split('\n') ?? [])

async function load(): Promise<void> {
  file.value = await store.loadSource(props.qn)
  // 11.15：实际边跳转时优先定位到调用点行。
  if (props.highlightLine) {
    // 11.15：先在元素树中选中包含该行的元素（不覆盖高亮），再把高亮收窄到调用点单行。
    syncTreeToLine(props.highlightLine)
    highlight.value = { start: props.highlightLine, end: props.highlightLine }
    void scrollToLine(props.highlightLine)
    return
  }
  const first = file.value?.elements?.[0]
  if (first) selectElement(first)
}

onMounted(() => void load())
watch(() => [props.qn, props.highlightLine], () => void load())

function selectElement(el: SourceElement): void {
  selectedQn.value = el.qn
  if (el.startLine) {
    highlight.value = { start: el.startLine, end: el.endLine ?? el.startLine }
    void scrollToLine(el.startLine)
  }
}

async function scrollToLine(line: number): Promise<void> {
  await nextTick()
  const pre = sourceRef.value
  if (!pre) return
  pre.scrollTop = Math.max(0, (line - 4) * 20)
}

function onSourceClick(evt: MouseEvent): void {
  const pre = sourceRef.value
  if (!pre) return
  const rect = pre.getBoundingClientRect()
  const line = Math.floor((evt.clientY - rect.top + pre.scrollTop) / 20) + 1
  syncTreeToLine(line)
}

function syncTreeToLine(line: number): void {
  const els = file.value?.elements ?? []
  const holder: { best: SourceElement | null } = { best: null }
  const walk = (list: SourceElement[]): void => {
    for (const el of list) {
      if (el.startLine && el.startLine <= line && (el.endLine ?? el.startLine) >= line) {
        if (!holder.best || (el.startLine ?? 0) >= (holder.best.startLine ?? 0)) holder.best = el
        if (el.children) walk(el.children)
      }
    }
  }
  walk(els)
  if (holder.best) {
    selectedQn.value = holder.best.qn
    if (holder.best.startLine) highlight.value = { start: holder.best.startLine, end: holder.best.endLine ?? holder.best.startLine }
  }
}

function startDrag(): void {
  dragging.value = true
  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', stopDrag)
}
function onDrag(evt: MouseEvent): void {
  const root = document.querySelector('.src-pane') as HTMLElement | null
  if (!root) return
  const rect = root.getBoundingClientRect()
  treeWidth.value = Math.max(140, Math.min(400, evt.clientX - rect.left))
}
function stopDrag(): void {
  dragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
}
</script>

<template>
  <div class="src-pane">
    <div v-if="file" class="pane-body">
      <!-- 左侧元素树 -->
      <div class="pane-tree" :style="{ width: treeWidth + 'px' }">
        <div class="tree-head" :title="qn">元素树（{{ file.elements.length }}）</div>
        <div class="tree-scroll">
          <div v-for="el in file.elements" :key="el.qn" class="tree-node depth-0">
            <div class="tn-row" :class="{ sel: selectedQn === el.qn }" @click="selectElement(el)">
              <span class="tn-kind" :style="{ background: kindColor[el.kind] ?? '#A0A0A0' }">{{ el.kind }}</span>
              <span v-if="el.isSugar" class="tn-sugar">sugar</span>
              <span class="tn-label">{{ el.label }}</span>
            </div>
            <div v-for="c in el.children ?? []" :key="c.qn" class="tree-node depth-1">
              <div class="tn-row" :class="{ sel: selectedQn === c.qn }" @click="selectElement(c)">
                <span class="tn-kind" :style="{ background: kindColor[c.kind] ?? '#A0A0A0' }">{{ c.kind }}</span>
                <span v-if="c.isSugar" class="tn-sugar">sugar</span>
                <span class="tn-label">{{ c.label }}</span>
                <span class="tn-line">:{{ c.startLine }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="pane-divider" :class="{ drag: dragging }" @mousedown="startDrag" />

      <!-- 右侧源码 -->
      <div class="pane-source">
        <div class="src-toolbar">
          <span class="src-path" :title="file.path">{{ file.path }}</span>
          <span class="src-lines">{{ file.lineCount }} 行</span>
          <span v-if="highlight" class="src-hl">L{{ highlight.start }}–{{ highlight.end }}</span>
        </div>
        <pre ref="sourceRef" class="src-pre" @click="onSourceClick">
          <div
            v-for="(ln, i) in lines"
            :key="i"
            class="src-line"
            :class="{ hl: highlight && i + 1 >= highlight.start && i + 1 <= highlight.end }"
          >
            <span class="ln">{{ i + 1 }}</span>
            <span class="code" v-html="(ln || ' ').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')" />
          </div>
        </pre>
      </div>
    </div>
    <div v-else class="pane-empty">加载中…</div>
  </div>
</template>

<style scoped>
.src-pane {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.pane-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}
.pane-empty {
  padding: 20px;
  color: #86909c;
  font-size: 12px;
}
.pane-tree {
  display: flex;
  flex-direction: column;
  min-width: 140px;
  background: #fafbfc;
  border-right: 1px solid #e5e6eb;
  flex-shrink: 0;
}
.tree-head {
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 600;
  color: #4e5969;
  border-bottom: 1px solid #e5e6eb;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree-scroll {
  flex: 1;
  overflow: auto;
}
.tree-node.depth-0 .tn-row {
  padding: 5px 12px;
  font-weight: 600;
}
.tree-node.depth-1 .tn-row {
  padding: 4px 12px 4px 24px;
  font-weight: 400;
}
.tn-row {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  font-size: 12px;
  color: #1f2329;
  border-left: 3px solid transparent;
}
.tn-row:hover {
  background: #f2f3f5;
}
.tn-row.sel {
  background: #e8f3ff;
  border-left-color: #165dff;
}
.tn-kind {
  padding: 1px 5px;
  border-radius: 4px;
  color: #fff;
  font-size: 10px;
  min-width: 40px;
  text-align: center;
  flex-shrink: 0;
}
.tn-sugar {
  padding: 0 5px;
  border-radius: 4px;
  background: #fff3e8;
  color: #ff7d00;
  font-size: 10px;
  flex-shrink: 0;
}
.tn-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tn-line {
  color: #c0c4cc;
  font-size: 11px;
  margin-left: auto;
}
.pane-divider {
  width: 5px;
  cursor: col-resize;
  background: #f2f3f5;
  flex-shrink: 0;
}
.pane-divider:hover,
.pane-divider.drag {
  background: #165dff;
}
.pane-source {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.src-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 12px;
  border-bottom: 1px solid #e5e6eb;
  font-size: 12px;
}
.src-path {
  font-family: monospace;
  color: #1f2329;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.src-lines {
  color: #86909c;
  flex-shrink: 0;
}
.src-hl {
  color: #165dff;
  margin-left: auto;
  flex-shrink: 0;
}
.src-pre {
  flex: 1;
  overflow: auto;
  margin: 0;
  padding: 4px 0;
  font-family: monospace;
  font-size: 12px;
  line-height: 20px;
  background: #fff;
}
.src-line {
  display: flex;
  padding: 0 12px;
  white-space: pre;
}
.src-line.hl {
  background: #fff7e6;
}
.ln {
  width: 42px;
  flex-shrink: 0;
  text-align: right;
  padding-right: 12px;
  color: #c0c4cc;
  user-select: none;
}
.code {
  color: #1f2329;
}
</style>
