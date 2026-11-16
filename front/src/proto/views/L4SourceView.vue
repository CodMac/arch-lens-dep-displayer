<script setup lang="ts">
// L3 源码视图（原型）：源码同步高亮 + 元素层级树 + sugar/kind 徽标。
// 11.14：支持双节点预览（边详情两端类）——qn2 存在时左右并排两个源码面板。
import SourcePane from '../components/SourcePane.vue'

const props = defineProps<{
  qn: string
  qn2?: string
  /** 11.15：调用点行号（实际边跳转时高亮）。 */
  highlightLine?: number
  srcQn?: string
  dstQn?: string
}>()
const emit = defineEmits<{ (e: 'back'): void }>()
</script>

<template>
  <div class="l4">
    <header class="l4-head">
      <button class="back-btn" @click="emit('back')">‹ 返回</button>
      <div class="l4-title">
        <h3>L3 · 源码与语法糖解构</h3>
        <span class="l4-sub">
          {{ props.qn2 ? '双节点预览' : '单节点' }}
        </span>
      </div>
    </header>

    <div class="l4-body" :class="{ dual: !!props.qn2 }">
      <SourcePane :qn="props.qn" :highlight-line="props.highlightLine" class="pane" />
      <template v-if="props.qn2">
        <div class="pane-gap" />
        <SourcePane :qn="props.qn2" class="pane" />
      </template>
    </div>

    <p class="l4-hint">
      点左树 → 右侧定位高亮对应行范围 · 点右侧源码行 → 左树同步选中元素 · 橙色徽标 = 语法糖（IsFormSugar）
      <span v-if="props.qn2"> · 双节点预览：左 = 源类，右 = 目标类</span>
    </p>
  </div>
</template>

<style scoped>
.l4 {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.l4-head {
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
.l4-title h3 {
  margin: 0;
  font-size: 16px;
}
.l4-sub {
  color: #86909c;
  font-size: 12px;
  font-family: monospace;
}
.l4-body {
  flex: 1;
  min-height: 0;
  display: flex;
  margin: 12px 20px;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
}
.l4-body.dual {
  border: none;
  background: transparent;
  gap: 0;
}
.l4-body.dual .pane {
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
}
.pane-gap {
  width: 10px;
  flex-shrink: 0;
}
.l4-hint {
  margin: 0 20px 12px;
  font-size: 12px;
  color: #86909c;
}
</style>
