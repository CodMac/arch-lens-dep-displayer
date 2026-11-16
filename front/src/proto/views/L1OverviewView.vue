<script setup lang="ts">
// L1 顶层鸟瞰（原型）：Combo 嵌套气泡 + 违规视觉 + 可切换 DSM 矩阵。
// 气泡视图（5.53 优化）：渐进式分层 + 画布内聚焦下钻 + 模块色相背景。
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import ProtoGraphCanvas from '../components/ProtoGraphCanvas.vue'
import { useProtoStore } from '../protoStore'
import {
  violationResultFromBackend, DEFAULT_VIOLATION_CONFIG, ALL_REL_TYPES, directPkg,
  type ViolationConfig, type ViolationResult, type BackendFanStat,
} from '../violation'
import { useDimensionStore } from '../../stores/dimension'
import {
  fetchViolationConfig, putViolationConfig, fetchFanStats, fetchViolations, type ViolationsResp,
} from '../../api/proto'
import type { OverviewPayload, L2Node, L2Edge, MemberInfo, EdgeCallDetail, DirectType, TypeEdge, ActualEdge } from '../types/model'

const store = useProtoStore()
const dimensionStore = useDimensionStore()
/** 可计入环的关系类型（维度配置下发，未下发时回落内置全集）。 */
const allRelTypes = computed<readonly string[]>(() =>
  dimensionStore.depRelationTypes.length ? dimensionStore.depRelationTypes : ALL_REL_TYPES,
)
const emit = defineEmits<{
  /** 查看源码 → L3（术语统一：L2=影响面/蝴蝶、L3=源码）。 */
  (e: 'go-source', qn: string): void
  /** 类节点 → L2 影响面（蝴蝶）。 */
  (e: 'go-blast', qn: string): void
  /** 11.19：方法 → 方法级 L2 Blast Radius（宿主类 qn + 方法 qn）。 */
  (e: 'go-blast-method', typeQn: string, methodQn: string): void
}>()

const payload = ref<OverviewPayload | null>(null)
const mode = ref<'bubble' | 'dsm'>('bubble') // 5.3 双模画布
const dsmSortBy = ref<'name' | 'strength'>('name') // 5.25 DSM 排序

const canvasRef = ref<InstanceType<typeof ProtoGraphCanvas>>()

/** 类级依赖边（包级边的汇聚源）：payload 的包级边是预聚合结果，已丢失「指向包内哪个类」的信息，
 *  故边汇聚改用类级边，可精确上卷到最近可见节点（包或类）。 */
const typeEdges = ref<TypeEdge[]>([])

onMounted(async () => {
  payload.value = await store.loadOverview()
  typeEdges.value = await store.loadTypeEdges()
  await syncViolationFromBackend()
  initFocus()
})

// ==================== 违规口径（11.16） ====================
// 高扇入/扇出：发生点固定在类（引用次数加权），阈值可配；包层只做子树向上汇聚。
// 循环依赖：类级边按类型过滤 + 嵌套类归并 → Tarjan → 类级 SCC → 直接所属包投影（仅跨包组）。
// 口径改动不自动生效：点「重算」才应用（draft → applied），避免调参过程中画布反复抖动。
const VIOLATION_CFG_KEY = 'arch-lens.violation-config'

function loadViolationConfig(): ViolationConfig {
  try {
    const raw = localStorage.getItem(VIOLATION_CFG_KEY)
    if (raw) {
      const v = JSON.parse(raw) as Partial<ViolationConfig>
      return {
        fanWarn: Number(v.fanWarn) > 0 ? Number(v.fanWarn) : DEFAULT_VIOLATION_CONFIG.fanWarn,
        fanSevere: Number(v.fanSevere) > 0 ? Number(v.fanSevere) : DEFAULT_VIOLATION_CONFIG.fanSevere,
        cycleTypes: Array.isArray(v.cycleTypes) && v.cycleTypes.length
          ? v.cycleTypes.filter((t) => allRelTypes.value.includes(t))
          : [...DEFAULT_VIOLATION_CONFIG.cycleTypes],
        mergeNested: v.mergeNested !== false,
      }
    }
  } catch {
    // 忽略损坏的本地配置，回落默认值
  }
  return { ...DEFAULT_VIOLATION_CONFIG, cycleTypes: [...DEFAULT_VIOLATION_CONFIG.cycleTypes] }
}

/** 编辑中的口径（面板表单绑定）。 */
const violationDraft = ref<ViolationConfig>(loadViolationConfig())
/** 已生效的口径（画布/详情/概览读取）。 */
const violationConfig = ref<ViolationConfig>(violationDraft.value)

/** 后端预计算违规数据（真实数据模式下的唯一事实来源）。 */
const backendFanStats = ref<BackendFanStat[]>([])
const backendViolations = ref<ViolationsResp | null>(null)

/** 真实后端：口径 + 违规结果均以后端预计算为准（口径在服务端，前端不再切分 QN）。 */
async function syncViolationFromBackend(): Promise<void> {
  const id = dimensionStore.currentId
  if (id == null) return
  try {
    const [c, fans, vio] = await Promise.all([
      fetchViolationConfig(id),
      fetchFanStats(id),
      fetchViolations(id),
    ])
    const next: ViolationConfig = {
      fanWarn: c.fanWarn > 0 ? c.fanWarn : DEFAULT_VIOLATION_CONFIG.fanWarn,
      fanSevere: c.fanSevere > 0 ? c.fanSevere : DEFAULT_VIOLATION_CONFIG.fanSevere,
      cycleTypes: c.cycleRelationTypes?.length
        ? c.cycleRelationTypes.filter((t) => allRelTypes.value.includes(t))
        : [...DEFAULT_VIOLATION_CONFIG.cycleTypes],
      mergeNested: c.mergeNested !== false,
    }
    violationDraft.value = next
    violationConfig.value = next
    backendFanStats.value = fans
    backendViolations.value = vio
  } catch {
    // 后端不可用时沿用本地配置，不阻塞画布
  }
}
/** 口径设置区是否展开。 */
const showViolationSettings = ref(false)
/** 重算提示（短暂显示）。 */
const recomputeHint = ref('')

const violationResult = computed<ViolationResult | null>(() => {
  const p = payload.value
  if (!p || !backendViolations.value) return null
  // 直接采用预计算结果（类级发生点 + 跨包环 + 宿主归一化）。
  return violationResultFromBackend(
    backendFanStats.value,
    backendViolations.value,
    p.tree,
    violationConfig.value,
  )
})

/** 应用编辑中的口径 → 重算并持久化。 */
function applyViolationConfig(): void {
  const d = violationDraft.value
  // 输入框清空时 v-model.number 会给出 NaN，回落到当前生效值。
  const rawWarn = Number.isFinite(d.fanWarn) ? d.fanWarn : violationConfig.value.fanWarn
  const rawSevere = Number.isFinite(d.fanSevere) ? d.fanSevere : violationConfig.value.fanSevere
  const warn = Math.max(1, Math.round(rawWarn))
  const severe = Math.max(warn, Math.round(rawSevere))
  if (d.fanWarn !== warn || d.fanSevere !== severe) {
    violationDraft.value = { ...d, fanWarn: warn, fanSevere: severe }
  }
  const next: ViolationConfig = {
    fanWarn: warn,
    fanSevere: severe,
    cycleTypes: [...d.cycleTypes],
    mergeNested: d.mergeNested,
  }
  violationConfig.value = next
  try {
    localStorage.setItem(VIOLATION_CFG_KEY, JSON.stringify(next))
  } catch {
    // 存储不可用不影响本次重算
  }
  recomputeHint.value = `已重算：警告 ≥${next.fanWarn} / 严重 ≥${next.fanSevere} · 环类型 ${next.cycleTypes.length} 种`
  window.setTimeout(() => { recomputeHint.value = '' }, 2400)
  // 口径落库并触发派生表重算（画布仍按上面本地口径即时重算）。
  const id = dimensionStore.currentId
  if (id != null) {
    void putViolationConfig(id, {
      fanWarn: next.fanWarn,
      fanSevere: next.fanSevere,
      cycleRelationTypes: next.cycleTypes,
      mergeNested: next.mergeNested,
    })
      .then((res) => {
        // 后端重算完成：用新结果替换画布数据源（阈值命中/环组以服务端为准）。
        backendViolations.value = res as ViolationsResp
      })
      .catch(() => {
        recomputeHint.value = '口径已本地生效，但后端保存失败'
        window.setTimeout(() => { recomputeHint.value = '' }, 2400)
      })
  }
}

/** 恢复默认口径（仅重置表单，需再点重算生效）。 */
function resetViolationDraft(): void {
  violationDraft.value = { ...DEFAULT_VIOLATION_CONFIG, cycleTypes: [...DEFAULT_VIOLATION_CONFIG.cycleTypes] }
}

function toggleCycleType(t: string): void {
  const set = new Set(violationDraft.value.cycleTypes)
  if (set.has(t)) set.delete(t)
  else set.add(t)
  violationDraft.value = { ...violationDraft.value, cycleTypes: [...set] }
}

/** 当前口径摘要（面板标题旁提示）。 */
const violationSummaryText = computed(() => {
  const c = violationConfig.value
  return `引用次数 ≥${c.fanWarn} / 严重 ≥${c.fanSevere} · 环 ${c.cycleTypes.length}/${allRelTypes.value.length} 类边${c.mergeNested ? ' · 嵌套类归并' : ''}`
})

// ==================== 聚焦下钻（5.53） ====================
// 业务根从数据推导：包树根出发沿「单子包链」下钻，第一个多子包层即业务根。
// 真实包树前三层（com→huawei→tenet）是纯命名空间，业务根=com.huawei.tenet，其 10 个直接子包=业务模块。
// 推导而非硬编码：换仓库（不同包结构）时自动适应。
const BUSINESS_ROOT = computed<string | null>(() => {
  const p = payload.value
  if (!p) return null
  // 真实数据可能有多个根（如无包声明元素落在 "(default)"）：取规模最大的根作为主干。
  const roots = p.tree.filter((t) => !t.parent)
  if (!roots.length) return null
  const sizeOf = (qn: string): number => p.nodes.find((n) => n.id === qn)?.typeCount ?? 0
  roots.sort((a, b) => sizeOf(b.qn) - sizeOf(a.qn))
  let cur = roots[0].qn
  while (cur) {
    const kids = pkgChildren(cur)
    if (kids.length !== 1) return cur // 第一个多子包层（或叶子）即业务根
    cur = kids[0]
  }
  return null
})

/** 主框集合（11.6 多主框重构）：所有已下钻展开的主框包。有序数组，末尾 = 最新激活的主框（主高亮）。
 *  11.9 解耦：主框只影响视觉（放大/高亮/nestLevel），不再决定节点增减。
 *  节点增减由 expandedPkgs（原则 2/3）驱动。 */
const mainBoxes = ref<string[]>([])

/** 已展开子节点的包集合（11.9 原则 2/3 的唯一读写点）：
 *  下钻 = add（增节点，子包显示）；折叠 = delete（减节点，直接子包及其子孙消失）。
 *  可见包树 = 业务根 ∪ { 每个展开包的直接子包 }。 */
const expandedPkgs = ref<Set<string>>(new Set())

/** 被折叠包集合：移除其子节点（含子孙）后自身缩小为本体（折叠壳）。 */
const collapsedPkgs = ref<Set<string>>(new Set())

function pkgChildren(qn: string): string[] {
  const p = payload.value
  if (!p) return []
  return p.tree.filter((t) => t.parent === qn).map((t) => t.qn)
}

function isLeaf(qn: string): boolean {
  return (payload.value?.nodes.find((n) => n.id === qn)?.children ?? 0) === 0
}

function initFocus(): void {
  mainBoxes.value = []
  expandedPkgs.value = new Set()
  collapsedPkgs.value = new Set()
}

/** 顶层业务模块包（业务根直接子包）。 */
const topModules = computed(() => {
  const p = payload.value
  if (!p) return []
  const br = BUSINESS_ROOT.value
  const kids = br ? pkgChildren(br) : []
  return kids.length ? kids : p.nodes.filter((n) => (n.depth ?? 0) === 0).map((n) => n.id)
})

function parentOf(qn: string): string {
  return payload.value?.tree.find((t) => t.qn === qn)?.parent ?? ''
}

/** 祖先链（含自身，自近向远）。 */
function ancestorChain(qn: string): string[] {
  const out: string[] = []
  let cur = qn
  while (cur) {
    out.push(cur)
    cur = parentOf(cur)
  }
  return out
}

/** a 是否是 anc 的后代（anc 是 a 的祖先）。 */
function isDescendantOf(a: string, anc: string): boolean {
  return a !== anc && ancestorChain(a).includes(anc)
}

/** 最新激活主框（末尾）。 */
const mainComboQn = computed(() => mainBoxes.value[mainBoxes.value.length - 1] ?? null)

/** 最新主框直接子包数（仅提示文案用；画布可见包树见 visiblePackages）。 */
const mainChildren = computed(() => {
  const main = mainComboQn.value
  return main ? pkgChildren(main) : topModules.value
})

/** 可见包树（11.9 原则 2/3 的显示集合，节点增减唯一依据）：
 *  业务根直接子包 ∪ 每个展开包的直接子包 ∪ 主框自身 ∪ 折叠壳。
 *  主框（mainBoxes）只影响视觉（nestLevel/放大/高亮），不参与此集合的加减（原则 4）。
 *  下钻 = expandedPkgs.add（增节点，直接子包进入此集合）；
 *  折叠 = expandedPkgs.delete（减节点，直接子包及其子孙离开此集合）。
 *  周边散列包（主框兄弟/祖先相关包）由「业务根直接子包 + 已展开上级主框的直接子包」自然覆盖，
 *  旧主框已展开的子包随 expandedPkgs 保留（不清除已加载节点，问题 2）。
 *  11.10：不再加入锚点目标深包（mainAnchorEdges 冗余，payload 边已含叶子包全部跨包依赖）。 */
const visiblePackages = computed(() => {
  const s = new Set<string>(topModules.value)
  for (const m of expandedPkgs.value) {
    for (const kid of pkgChildren(m)) s.add(kid)
  }
  for (const m of mainBoxes.value) s.add(m)
  for (const c of collapsedPkgs.value) s.add(c)
  return s
})

/** 焦点区包（11.9）：无主框 = 业务根直接子包（顶层视图）；有主框 = 所有主框自身 + 其直接子包。
 *  焦点区包画 combo/node 正常形态归入各自主框；非焦点区包是周边散列（node 小点）。
 *  与 11.6 的区别：遍历**所有**主框（而非仅最新），旧主框的直接子包保留（问题 2：不清除已加载节点）。 */
const focusPkgs = computed(() => {
  const s = new Set<string>()
  if (!mainBoxes.value.length) {
    for (const m of topModules.value) s.add(m)
    return s
  }
  for (const m of mainBoxes.value) {
    s.add(m)
    for (const kid of pkgChildren(m)) s.add(kid)
  }
  return s
})

/** 周边散列包：可见包中不属于焦点区的包（主框兄弟模块、祖先相关上下文），缩小散列在对应祖先框内（node 形态）。 */
const outsidePkgs = computed(() => {
  const fp = focusPkgs.value
  return [...visiblePackages.value].filter((qn) => !fp.has(qn))
})

/** 下钻（11.9 原则 2：增节点）——解耦为两步：
 *  ① expandedPkgs.add(id)：该包直接子包变为可见（增节点）；
 *  ② mainBoxes 重排（纯视觉聚焦）：兄弟/无关切换旧主框保留缩小、父切子旧主框保留嵌套、
 *     子切父/切祖先移除该包的所有子孙主框（丢弃子孙，expandedPkgs 也同步删，见下方注释）。
 *  主框切换（已展开包重激活，expandedPkgs 已含）→ 只走 ②，节点零变化（原则 4）。 */
function drillTo(id: string): void {
  if (id === mainComboQn.value) return // 已是最新主框，不动作
  selectPkg(id)
  // ① 展开（增节点）。expandedPkgs 与 collapsedPkgs 互斥：展开即清除折叠标记。
  const e = new Set(expandedPkgs.value)
  e.add(id)
  expandedPkgs.value = e
  const c = new Set(collapsedPkgs.value)
  c.delete(id) // 重新展开
  collapsedPkgs.value = c
  // ② 主框聚焦（纯视觉）。子切父/切祖先：丢弃该包子孙主框的同时，同步删其展开标记
  //   （子孙子包不再显示，符合原则 3 折叠语义）；父切子/兄弟切换：旧主框保留，展开标记不动。
  const keep = mainBoxes.value.filter((m) => !isDescendantOf(m, id))
  const dropped = mainBoxes.value.filter((m) => isDescendantOf(m, id))
  mainBoxes.value = [...keep.filter((m) => m !== id), id]
  if (dropped.length) {
    const e2 = new Set(expandedPkgs.value)
    for (const d of dropped) e2.delete(d)
    expandedPkgs.value = e2
  }
  void nextTick(() => canvasRef.value?.focusElement(id))
}

/** 折叠（11.9 原则 3：减节点）：expandedPkgs.delete(id) → 直接子包及其子孙全部消失（减节点）；
 *  collapsedPkgs.add(id) → 自身缩小为本体（折叠壳标记，子节点不再显示）。
 *  主框移除自身及其子孙主框（不再聚焦已收起的包）；drillTo 重新展开时清除折叠标记。 */
function collapsePkg(id: string): void {
  if (isLeaf(id)) return // 叶子无子节点可折叠
  const e = new Set(expandedPkgs.value)
  e.delete(id)
  expandedPkgs.value = e
  // 折叠壳：自身保留显示（collapsed 标记），子节点已随 expandedPkgs.delete 移除。
  const c = new Set(collapsedPkgs.value)
  c.add(id)
  collapsedPkgs.value = c
  // 移除该包及其所有子孙主框（其内容不再展开），叶子数据随 leafLoadTargets watch 重建。
  mainBoxes.value = mainBoxes.value.filter((m) => m !== id && !isDescendantOf(m, id))
  if (selectedQn.value === id) selectedQn.value = null
  void nextTick(() => canvasRef.value?.fitView())
}

/** 画布可见全部 id（焦点区 + 周边 + 可见类），边汇聚/环高亮的判定集合。
 *  11.9：可见包树 = visiblePackages（原则 2/3 驱动）；主框只影响视觉（原则 4）。
 *  11.12：加入非叶子主框的直接类节点——类级边上卷时可精确落到类（如 kgcore.dao → kgcore.IpUtils），
 *  而非停在已展开的包（kgcore）上。 */
const visibleAll = computed(() => {
  const s = new Set<string>(visiblePackages.value)
  for (const n of directTypeNodes.value) s.add(n.id)
  return s
})

/** 可折叠包集合（交互收敛：折叠对任意可见包生效）：可见包全集（不含叶子，叶子无子节点可折叠）。
 *  右键菜单「折叠」项据此显示；折叠 = 丢弃直接子包及其子孙，自身缩小保留（无虚线壳）。 */
const collapsibleIds = computed<Set<string>>(() => {
  const s = new Set<string>()
  for (const qn of visibleAll.value) if (!isLeaf(qn)) s.add(qn)
  return s
})

/** combo/节点归位（11.6 多主框 + 11.9 可见树）：基于「所有主框」的可见包树归位。
 *  每个主框 → 归入其父框（若父框也是主框/祖先链容器）→ 根；
 *  主框的直接子包 → 其主框（含旧主框的已展开子包，问题 2：不清除已加载节点）。
 *  周边散列包：是某主框的直接子包 → 归入该主框；是某主框的兄弟/祖先 → 按最近主框祖先嵌套。
 *  形成「业务根 → 模块 → … → 父框 → 主框」逐层嵌套，主框被父框包住，父框被祖父框包住。
 *  被折叠包（collapsedPkgs）作为本体保留，其子包不再显示（collapsePkg 已从 mainBoxes 移除）。 */
function comboParentOf(qn: string): string {
  // 是主框 → 归入其父框（若父框也是主框/祖先链容器）；业务根直接子包主框 → 根。
  if (mainBoxes.value.includes(qn)) {
    const par = parentOf(qn)
    if (!par) return ''
    // 父框在主框集合或作为祖先链容器存在 → 嵌套；否则父框未展开（非主框）→ 归入最近主框祖先。
    if (mainBoxes.value.includes(par)) return par
    for (let i = mainBoxes.value.length - 1; i >= 0; i--) {
      const m = mainBoxes.value[i]
      if (isDescendantOf(par, m)) return m
    }
    return ''
  }
  // 主框直接子包 → 其主框。
  for (const m of mainBoxes.value) {
    if (pkgChildren(m).includes(qn)) return m
  }
  // 周边散列包：是某主框的兄弟包 → 归入该主框的父框（若父框展开）；否则归根。
  for (const m of mainBoxes.value) {
    const par = parentOf(m)
    if (par && pkgChildren(par).includes(qn)) return mainBoxes.value.includes(par) ? par : ''
  }
  // 兜底：最近主框祖先。
  for (let i = mainBoxes.value.length - 1; i >= 0; i--) {
    const m = mainBoxes.value[i]
    if (isDescendantOf(qn, m)) return m
  }
  return ''
}

/** 包节点 → combo（短名 label + 完整 QN 副文本 + 环/高扇入/违规计数标记）。
 *  opts.leaf = 叶子包也建 combo（叶子包就地展开类的容器，L1/L2 合并 §2.2）。
 *  11.13：移除「色块 = 业务模块」设计——模块色与环/高扇入描边抢视觉，combo 回归中性 depth 分档。
 *  11.5（#57）：nestLevel = 离主框的层级距离（主框=0，父框=1，祖父框=2…），
 *  周边散列包按其所在层的距离算；用于画布侧按层级递进缩小 combo 尺寸。 */
function toCombo(qn: string, opts?: { leaf?: boolean }): { id: string; label: string; qn?: string; parent: string; size: number; depth: number; nestLevel: number; childCount: number; collapsed?: boolean; cycleKind?: number; highFanin?: boolean; violation?: { cycle: number; fanin: number; fanout: number } } | null {
  const p = payload.value
  if (!p) return null
  const n = p.nodes.find((x) => x.id === qn)
  if (!n) return null
  const isLeaf = (n.children ?? 0) === 0
  if (isLeaf && !opts?.leaf) return null
  const depth = n.depth ?? 0
  const ck = cycleKindOf.value[qn]
  // 违规向上汇聚（11.16）：子树内环组数 / 高扇出类数，供计数角标。
  const vr = violationResult.value
  const cycCnt = vr?.subtreeCycleCount?.[qn] ?? 0
  const faninCnt = vr?.subtreeFaninCount?.[qn] ?? 0
  const fanoutCnt = vr?.subtreeFanoutCount?.[qn] ?? 0
  // 11.17：高扇入与高扇出可同时存在，两个计数角标都展示（原只带 fanout）。
  const violation =
    cycCnt > 0 || faninCnt > 0 || fanoutCnt > 0
      ? { cycle: cycCnt, fanin: faninCnt, fanout: fanoutCnt }
      : undefined
  // 短名 label（决策 6 方案 B 第一行）；有子包时加子包数角标；完整 QN 由 qn 字段承载（灰色副文本）。
  // 11.1 问题2：周边包与主框短名重复时（如主框 mongo vs dao.mongo），
  // label 用「父短名/短名」区分（dao/mongo），消除「边指向同名包=像指向不存在点」的视觉歧义。
  const shortName = n.label ?? qn.split('.').pop() ?? qn
  const mainShort = mainComboQn.value?.split('.').pop() ?? ''
  const segs = qn.split('.')
  const parentSeg = segs.length >= 2 ? segs[segs.length - 2] : ''
  const disambiguated = shortName === mainShort && qn !== mainComboQn.value && parentSeg
    ? `${parentSeg}/${shortName}`
    : shortName
  const label = disambiguated + (isLeaf ? '' : ` (${n.children})`)
  // 11.6 多主框：nestLevel = 离最新主框的层级距离。主框=0，父框=1，祖父框=2…
  // 周边散列包按其所在层距离算；用于画布侧按层级递进缩小 combo 尺寸。
  let nestLevel = 0
  const main = mainComboQn.value
  if (main) {
    if (qn === main) nestLevel = 0
    else if (mainBoxes.value.includes(qn)) {
      // 旧主框：到最新主框的最近公共祖先层级距离。
      const lcaChain = ancestorChain(main).filter((a) => isDescendantOf(a, qn) || a === qn)
      nestLevel = lcaChain.length ? lcaChain.length : 1
    } else {
      // 周边/子包：距最新主框的层级距离（沿祖先链计数）。
      const chain = ancestorChain(qn)
      const idx = chain.indexOf(main)
      nestLevel = idx >= 0 ? idx + 1 : chain.length
    }
  }
  return {
    id: qn,
    label,
    qn,
    parent: comboParentOf(qn),
    size: n.size,
    depth,
    nestLevel,
    childCount: n.children ?? 0,
    // 折叠壳：自身缩小为本体（虚线描边 + collapsedMarker），子节点不再显示。
    collapsed: collapsedPkgs.value.has(qn),
    ...(ck ? { cycleKind: ck } : {}),
    ...(highFaninSet.value.has(qn) ? { highFanin: true } : {}),
    ...(highFanoutSet.value.has(qn) ? { highFanout: true } : {}), // 11.7：高扇出包成 combo 后仍需绿描边
    ...(violation ? { violation } : {}),
  }
}

/** 可见 combo（11.6 多主框 + 11.8 问题4 + 11.9 可见树）：所有主框 + 所有主框内非叶子子包。
 *  焦点区子包（非叶子）按 comboParentOf 归位到对应主框（含旧主框已展开子包，问题 2）；
 *  直接子层叶子包为 node 单点（不建容器 combo）；周边散列包不建 combo（node 形态，见 visibleLeafNodes）。
 *  11.10：不再建锚点目标深包 combo（payload 边已覆盖叶子包跨包依赖，上卷到可见祖先）。 */
const visibleCombos = computed(() => {
  // 折叠的周边散列包（node 形态，见 visibleLeafNodes）不在此列——自身即本体，避免 node/combo 身份重叠。
  const nodeForm = new Set(outsidePkgs.value)
  const childPkgs = [...focusPkgs.value].filter((qn) => !mainBoxes.value.includes(qn)) // 所有主框的直接子包
  const qns = [...new Set([
    ...mainBoxes.value, // 所有主框（含祖先链容器）
    ...childPkgs.filter((qn) => !isLeaf(qn)), // 所有主框内非叶子子包建 combo
    ...[...collapsedPkgs.value].filter((qn) => !nodeForm.has(qn)), // 折叠壳：主框/子包折叠自身保留显示（子节点已移除）
  ])]
  return qns.map((qn) => toCombo(qn, { leaf: mainBoxes.value.includes(qn) })).filter((x) => x !== null) as NonNullable<ReturnType<typeof toCombo>>[]
})

/** 可见 node（11.6 多主框 + 11.8 问题4 + 11.9 可见树）：所有主框直接子层叶子包（单点）+ 周边散列包全部。
 *  11.8 起主框直接子层叶子包恢复为 node 单点（不再建容器 combo 就地展开类）。
 *  周边散列包改 node 形态：combo-combined 布局对空 combo（无子元素）碰撞尺寸为 0 导致互相重叠，
 *  改 node 后按 data.size 参与 force 碰撞防重叠，且符合用户「祖先相关节点忽略权重统一变小」。
 *  11.10：锚点目标深包不再画成游离 node（payload 边已覆盖，上卷到可见祖先）。 */
const visibleLeafNodes = computed(() => {
  const childPkgs = [...focusPkgs.value].filter((qn) => !mainBoxes.value.includes(qn)) // 所有主框的直接子包
  const qns = [...childPkgs.filter((qn) => isLeaf(qn)), ...outsidePkgs.value]
    .filter((qn) => !mainBoxes.value.includes(qn)) // 主框包（combo 容器）不在此列，避免 node/combo 身份重叠
  // 折叠的周边散列包（node 形态）保留显示：其子节点从未单独渲染，折叠后自身即本体（无虚线壳）。
  return qns.map((qn) => {
    const n = payload.value?.nodes.find((x) => x.id === qn)!
    return { ...n, parent: comboParentOf(qn) }
  })
})

// ---- L1 类节点混合显示（L1/L2 合并，设计 §2.2/§3/§4） ----
// 渐进式类展开：主框聚焦时，其「直接子层的叶子包」就地展开类（每个叶子包建容器 combo，类归入其中）；
// 主框自身是叶子包时（下钻到底），就地显示其类（= 旧 L2 单包核心内容，不再独立成页）。
// 类数据按需拉取（loadLeafTypes），主框/聚焦层变化时重新拉取。
interface LeafTypeData {
  nodes: (L2Node & { parent: string })[]
  edges: L2Edge[]
  /** 类成员（类详情看板 2 tab：类详情 + 成员）。 */
  members: Record<string, MemberInfo[]>
}
const leafData = ref<Record<string, LeafTypeData>>({})

/** 需要加载类数据的包集合：仅主框自身为叶子包时（下钻到底），类直接显示在主框内。
 *  11.8（问题4）：叶子包不再就地展开类——主框直接子层叶子包恢复为 node 单点（11.4 #55 行为），
 *  双击叶子包才下钻（其类显示在叶子包主框内）。减少复杂度与布局飘移（11.7 问题2 被否定）。 */
function leafLoadTargets(): { qn: string; asCombo: boolean }[] {
  const out: { qn: string; asCombo: boolean }[] = []
  for (const m of mainBoxes.value) {
    if (isLeaf(m)) out.push({ qn: m, asCombo: false }) // 主框叶子：类直接显示在主框内
  }
  return out
}

// ---- 非叶子主框的直接类（11.11：包下第一层类，与子包并列展示） ----
// 如 kgcore 下钻后除 7 个子包外，还显示其直接类 IpUtils / KGCoreApplication。
// 数据按需拉取（loadDirectTypes），主框集合变化时重新拉取。
const directTypeData = ref<Record<string, DirectType[]>>({})

/** 需要加载直接类的包集合：非叶子主框（叶子主框走 leafData 的类数据）。 */
function directTypeLoadTargets(): string[] {
  return mainBoxes.value.filter((m) => !isLeaf(m))
}

let directTypeSeq = 0
watch(
  () => directTypeLoadTargets().join('|'),
  async (key) => {
    const seq = ++directTypeSeq
    if (!key) {
      directTypeData.value = {}
      return
    }
    const out: Record<string, DirectType[]> = {}
    for (const qn of key.split('|')) out[qn] = await store.loadDirectTypes(qn)
    if (seq === directTypeSeq) directTypeData.value = out
  },
  { immediate: true },
)

/** 非叶子主框的直接类节点（归入主框 combo，与子包并列）。 */
const directTypeNodes = computed<(L2Node & { parent: string })[]>(() =>
  Object.entries(directTypeData.value).flatMap(([pkg, list]) =>
    list.map((c) => ({
      id: c.id,
      label: c.label,
      kind: c.kind,
      inPkg: true,
      parent: pkg,
      memberCount: c.members.length,
      size: c.members.length,
    })),
  ),
)

/** 非叶子主框直接类的成员表（类详情「成员」tab 数据源）。 */
const directTypeMembers = computed<Record<string, MemberInfo[]>>(() => {
  const out: Record<string, MemberInfo[]> = {}
  for (const list of Object.values(directTypeData.value)) {
    for (const c of list) out[c.id] = c.members
  }
  return out
})

// 主框变化 → 重新拉取需展开的叶子包类数据（按需，仅主框聚焦时）。
// 异步竞态防护：快速连续下钻时多次 watch 回调并发，用序列号保证只有最新一次的结果生效
// （否则旧主框阶段的结果后完成会覆盖新主框的 leafData，导致类数据错位）。
let leafLoadSeq = 0
watch(
  () => leafLoadTargets().map((t) => t.qn).join('|'),
  async (key) => {
    const seq = ++leafLoadSeq
    if (!key) {
      leafData.value = {}
      return
    }
    const out: Record<string, LeafTypeData> = {}
    for (const qn of key.split('|')) {
      const pl = await store.loadLeafTypes(qn)
      const nodes = (pl?.nodes ?? []).filter((n) => n.inPkg).map((n) => ({ ...n, parent: qn }))
      const edges = (pl?.edges ?? []).filter((e) => {
        const s = (pl?.nodes ?? []).some((n) => n.id === e.source && n.inPkg)
        const t = (pl?.nodes ?? []).some((n) => n.id === e.target && n.inPkg)
        return s && t // 两端都是本包子类（内部类边）
      })
      const members = pl?.members ?? {}
      out[qn] = { nodes, edges, members }
    }
    if (seq === leafLoadSeq) leafData.value = out // 仅最新一次结果生效
  },
  { immediate: true },
)
// dev 调试钩子：暴露 leafData 键集 + 画布数据特征，供 puppeteer 验证（DEV only）。
if (import.meta.env.DEV) {
  watch(leafData, (d) => {
    ;(window as unknown as Record<string, unknown>).__leafDataKeys = Object.keys(d)
    ;(window as unknown as Record<string, unknown>).__leafDataMemberCount = Object.values(d).reduce((a, l) => a + Object.keys((l as LeafTypeData).members).length, 0)
  })
}

/** 已展开叶子包的类节点（仅主框叶子自身的类；11.8 问题4 起直接子层叶子不再就地展开）。 */
const leafTypeNodes = computed<(L2Node & { parent: string })[]>(() =>
  Object.values(leafData.value).flatMap((l) => l.nodes),
)
/** 已展开叶子包的类间边。 */
const leafTypeEdges = computed<L2Edge[]>(() => Object.values(leafData.value).flatMap((l) => l.edges))

/** L1 画布全部节点：叶子包节点（直接子层叶子单点 + 周边叶子）+ 已展开叶子包主框的类节点
 *  + 非叶子主框的直接类节点（11.11，与子包并列）。 */
const allCanvasNodes = computed(() => {
  const pkgNodes = visibleLeafNodes.value
  if (!mainComboQn.value) return pkgNodes // 顶层无主框：纯包视图
  return [...pkgNodes, ...leafTypeNodes.value, ...directTypeNodes.value]
})

/** 画布上全部类节点（叶子主框类 + 非叶子主框直接类），供点击命中/详情/端点溯源。 */
const allTypeNodes = computed<(L2Node & { parent: string })[]>(() => [
  ...leafTypeNodes.value,
  ...directTypeNodes.value,
])

/** 上卷函数：任意 QN（包/类/嵌套类型）→ 最近可见节点（包或可见类）。
 *  包（在包树中）沿父包上卷；类/嵌套类型（不在包树中）去最后一段到宿主，再继续上卷。 */
const rollupToVisible = computed<((qn: string) => string)>(() => {
  const p = payload.value
  const ids = visibleAll.value
  const parent = new Map((p?.tree ?? []).map((t) => [t.qn, t.parent]))
  return (qn: string): string => {
    let cur = qn
    while (cur && !ids.has(cur)) {
      cur = parent.has(cur)
        ? (parent.get(cur) ?? '')
        : (cur.includes('.') ? cur.slice(0, cur.lastIndexOf('.')) : '')
    }
    return cur
  }
})

// 可见边：按可见前沿汇聚（11.12 改用类级边为汇聚源）。
// 旧实现从 payload 包级边上卷：包级边是预聚合结果，端点只到包，无法表达「指向包内哪个类」，
// 导致展开包后仍出现 dao→kgcore（应指向 kgcore 内的 IpUtils 或子包）。
// 现从类级边上卷：端点先到类（若类可见，如非叶子主框的直接类）→ 否则到最近可见包祖先。
// 同一可见节点内为内部边丢弃；否则生成 节点→节点 汇聚边，count 累加。
const visibleEdges = computed(() => {
  const va = rollupToVisible.value
  const acc = new Map<string, { source: string; target: string; count: number; types: Set<string> }>()
  for (const e of typeEdges.value) {
    const s = va(e.source)
    const t = va(e.target)
    if (!s || !t || s === t) continue // 无可见祖先或同一可见节点内内部边
    const key = s + '->' + t
    const hit = acc.get(key)
    if (hit) {
      hit.count += e.count ?? 1
      for (const ty of e.types ?? []) hit.types.add(ty)
    } else {
      acc.set(key, { source: s, target: t, count: e.count ?? 1, types: new Set(e.types ?? []) })
    }
  }
  return [...acc.values()].map((v) => ({ source: v.source, target: v.target, type: 'DEPEND', count: v.count, types: [...v.types] }))
})

/** 画布可见边 = 包级汇聚边 + 叶子包类间明细边。
 *  11.10：去锚点汇聚边移除——叶子包跨包依赖已由 payload 边经 visibleEdges 上卷到可见祖先。 */
const allCanvasEdges = computed(() => {
  const pkgEdges = visibleEdges.value
  if (!mainComboQn.value) return pkgEdges
  return [...pkgEdges, ...leafTypeEdges.value]
})

/** 包间环边标记（11.16 改口径）：两端可见节点同属某类级环组（按直接所属包投影）→ 红实线。 */
const cycleEdgeKeys = computed<Set<string>>(() => {
  const s = new Set<string>()
  const r = violationResult.value
  if (!r) return s
  for (const e of allCanvasEdges.value) {
    const a = r.pkgCycle[e.source] ?? r.typeCycle[e.source]
    const b = r.pkgCycle[e.target] ?? r.typeCycle[e.target]
    if (a?.length && b?.length && a.some((id) => b.includes(id))) {
      s.add(`${e.source}->${e.target}`)
    }
  }
  return s
})

// ---- 缺陷概览面板（11.16 改口径）：类级发生点 + 阈值筛选 + 子树向上汇聚 ----
/** 可见范围内的高扇入/高扇出类（按引用次数加权，阈值来自当前口径）。
 *  「可见」= 宿主包自身可见，或其某个祖先包可见（祖先展开时其子树在画布范围内）。 */
const defectFanList = computed(() => {
  const r = violationResult.value
  if (!r) return { fanin: [], fanout: [], severeIn: 0, severeOut: 0 }
  const labelOf = (qn: string): string => qn.split('.').pop() ?? qn
  const pkgVisible = (pkg: string): boolean => ancestorChain(pkg).some((a) => visibleAll.value.has(a))
  const rows = (m: Map<string, number>, set: Set<string>, classes: Map<string, number>, kind: 'in' | 'out') =>
    [...set]
      .filter((qn) => pkgVisible(r.pkgOf[qn] ?? directPkg(qn)))
      .sort((a, b) => (m.get(b) ?? 0) - (m.get(a) ?? 0))
      .map((qn) => ({
        qn,
        label: labelOf(qn),
        value: m.get(qn) ?? 0,
        classes: classes.get(qn) ?? 0,
        kind,
        isType: true,
        severe: (r.config.fanSevere > 0) && (m.get(qn) ?? 0) >= r.config.fanSevere,
      }))
  return {
    fanin: rows(r.fanIn, r.highFanin, r.fanInClasses, 'in'),
    fanout: rows(r.fanOut, r.highFanout, r.fanOutClasses, 'out'),
    severeIn: r.severeFanin.size,
    severeOut: r.severeFanout.size,
  }
})

/** 环链路：类级 SCC（已归一化 + 跨包）的最短环（类级真实边，BFS）。
 *  链路始终按类展示（环中的点 = 类），与当前展开无关；`visible` 标记该环在当前画布是否可见，
 *  不可见时点击会自动下钻到宿主包。 */
const defectCyclePaths = computed(() => {
  const r = violationResult.value
  if (!r) return []
  const labelOf = (qn: string): string => qn.split('.').pop() ?? qn
  const pkgVisible = (pkg: string): boolean => ancestorChain(pkg).some((a) => visibleAll.value.has(a))
  const out: {
    group: number
    nodes: string[]
    labels: string[]
    kind: 'cross'
    pkgs: string[]
    classes: string[]
    visible: boolean
  }[] = []
  for (const c of r.cycles) {
    const adj = new Map<string, string[]>()
    for (const h of c.hops) {
      const arr = adj.get(h.source)
      if (arr) arr.push(h.target)
      else adj.set(h.source, [h.target])
    }
    let best: string[] | null = null
    for (const [s, targets] of adj) {
      for (const t of targets) {
        const prev = new Map<string, string | null>([[t, null]])
        const queue = [t]
        let found = false
        while (queue.length) {
          const cur = queue.shift()!
          if (cur === s) { found = true; break }
          for (const nx of adj.get(cur) ?? []) {
            if (!prev.has(nx)) { prev.set(nx, cur); queue.push(nx) }
          }
        }
        if (!found) continue
        const back: string[] = []
        let x: string | null = s
        while (x !== null) { back.push(x); x = prev.get(x) ?? null }
        back.reverse()
        const cycle = [s, ...back]
        if (!best || cycle.length < best.length) best = cycle
      }
    }
    if (!best) continue
    out.push({
      group: c.id,
      nodes: best,
      labels: best.map(labelOf),
      kind: 'cross',
      pkgs: c.pkgMembers,
      classes: c.typeMembers,
      visible: c.pkgMembers.every(pkgVisible),
    })
  }
  return out
})

/** 缺陷概览面板是否展开。 */
const showDefectPanel = ref(true)

/** 11.17：缺陷概览点击 = 纯查看详情，**不改变画布**（不下钻、不切换主框）。
 *  类不在当前画布时，用宿主包 + 源码包元素类型构造一个临时类节点供详情面板展示；
 *  需要看它在画布的位置时，由详情面板的「在画布中定位」按钮显式触发下钻。 */
let defectClickSeq = 0
async function onDefectItemClick(qn: string): Promise<void> {
  const seq = ++defectClickSeq
  selectedEdge.value = null
  const cls = allTypeNodes.value.find((n) => n.id === qn)
  if (cls) {
    selectedQn.value = null
    selectedType.value = cls
    return
  }
  if (payload.value?.nodes.some((n) => n.id === qn)) {
    selectedType.value = null
    selectedQn.value = qn
    return
  }
  // 未渲染的类：构造临时节点（kind 从源码包元素树取），详情照常展示。
  const pkg = violationResult.value?.pkgOf[qn] ?? directPkg(qn)
  const kind = (await store.loadTypeKind(qn)) as L2Node['kind']
  if (seq !== defectClickSeq) return // 期间用户已点别的项，丢弃过期结果
  selectedQn.value = null
  selectedType.value = {
    id: qn,
    label: qn.split('.').pop() ?? qn,
    kind,
    inPkg: true,
    parent: pkg,
    memberCount: undefined,
  }
}

/** 待定位的类：下钻后其直接类数据是异步加载的，等类节点出现再选中。 */
const pendingLocateQn = ref<string | null>(null)

function selectTypeNode(qn: string): boolean {
  const c = allTypeNodes.value.find((n) => n.id === qn)
  if (!c) return false
  selectedQn.value = null
  selectedType.value = c
  return true
}

// 类节点出现（直接类异步加载完成）→ 完成待定位。
watch(allTypeNodes, (nodes) => {
  const q = pendingLocateQn.value
  if (!q) return
  if (nodes.some((n) => n.id === q)) {
    pendingLocateQn.value = null
    selectTypeNode(q)
  }
})

/** 类详情「在画布中定位」（11.18）：**沿祖先链逐层下钻**到宿主包，再选中该类。
 *  不能直接 drillTo(宿主包)：若当前主框与宿主包无祖先关系，宿主包会以根级 combo 出现，
 *  与它本应嵌套的祖先包（如 common）变成两个互相独立的节点。逐层下钻可保证每一级祖先
 *  都进入 mainBoxes，从而按 comboParentOf 正确嵌套。 */
function locateTypeOnCanvas(qn: string): void {
  const pkg = violationResult.value?.pkgOf[qn] ?? directPkg(qn)
  if (!pkg) return
  pendingLocateQn.value = qn
  // root → pkg 的祖先链。
  const chain = ancestorChain(pkg).reverse()
  // 起点 = 当前画布已打开的最高祖先（主框或可见包）；都没有则从业务根直接子包起。
  let start = chain.findIndex((a) => mainBoxes.value.includes(a) || visiblePackages.value.has(a))
  if (start < 0) {
    const br = BUSINESS_ROOT.value
    start = chain.findIndex((a) => (br ? parentOf(a) === br : true))
    if (start < 0) start = chain.length - 1
  }
  for (const step of chain.slice(start)) {
    if (step === mainComboQn.value) continue // 已是当前主框，drillTo 会早退
    drillTo(step)
  }
  void nextTick(() => {
    if (pendingLocateQn.value === qn) selectTypeNode(qn)
  })
}

/** 11.19：成员是否可进入方法级影响面（仅方法/构造器；字段无调用点数据）。 */
function isMethodMember(m: MemberInfo): boolean {
  return m.kind === 'METHOD' || m.kind === 'CONSTRUCTOR'
}

/** 11.19：方法级 L2 —— 宿主类 qn + 方法 qn（路由 ?m=）。 */
function openMethodBlast(methodQn: string): void {
  const c = selectedType.value
  if (!c) return
  emit('go-blast-method', c.id, methodQn)
}

/** 当前选中的类是否已在画布上渲染（决定是否显示「在画布中定位」）。 */
const selectedTypeOnCanvas = computed<boolean>(() => {
  const c = selectedType.value
  if (!c) return false
  return allTypeNodes.value.some((n) => n.id === c.id)
})

/** 当前高亮的环链路（点击缺陷概览的环链路 → 画布整条环加粗高亮）。 */
const highlightedCycleNodes = ref<Set<string>>(new Set())
const highlightedCycleEdges = ref<Set<string>>(new Set())

/** 环链路点击 → 高亮整条环（按可见前沿上卷到画布节点）+ 选中链路首节点详情；
 *  再次点击同一条取消高亮。环成员当前不可见时先下钻到其宿主包。 */
function onDefectCycleClick(c: { nodes: string[]; visible?: boolean }): void {
  if (!c.nodes.length) return
  const va = rollupToVisible.value
  // 链路 nodes 是闭合环（末节点回到首节点），相邻对即环边。
  const edges = new Set<string>()
  const nodes = new Set<string>()
  for (let i = 0; i < c.nodes.length; i++) {
    const s = c.nodes[i]
    const t = c.nodes[(i + 1) % c.nodes.length]
    const vs = va(s)
    const vt = va(t)
    if (vs) nodes.add(vs)
    if (vt) nodes.add(vt)
    if (vs && vt && vs !== vt) edges.add(`${vs}->${vt}`)
  }
  const same = highlightedCycleNodes.value.size === nodes.size
    && [...nodes].every((n) => highlightedCycleNodes.value.has(n))
  if (same) {
    highlightedCycleNodes.value = new Set()
    highlightedCycleEdges.value = new Set()
  } else {
    highlightedCycleNodes.value = nodes
    highlightedCycleEdges.value = edges
  }
  if (c.visible === false) {
    // 环成员不在当前画布：只展示详情，不自动下钻（11.17：缺陷概览点击不改变画布）。
    // 用户可在类详情里点「在画布中定位」显式下钻。
  }
  void onDefectItemClick(c.nodes[0]!)
}

// ---- 右侧节点细节信息框（决策 7） ----
// 点击任意包（combo / 叶子节点）→ 选中并显示细节面板：完整 QN、kind、子包数、类规模、
// 所属模块、扇入/扇出（包数与边条数）、环归属、高扇入标记。面板内提供下钻 / 进 L2 操作。
const selectedQn = ref<string | null>(null)
/** 主框内被点击的类节点（决策 Q4 包+类混合显示）。 */
const selectedType = ref<(L2Node & { parent: string }) | null>(null)
/** 类详情看板 tab（#40）：detail=类详情 / members=成员列表。 */
const typeDetailTab = ref<'detail' | 'members'>('detail')

/** 11.16：类的违规标记（发生点在类，向上汇聚看宿主包）。
 *  环 = 类自身所在环组 ∪ 宿主包投影到的环组；高扇入/扇出 = 类自身阈值命中。 */
const selectedTypeCycle = computed<number[]>(() => {
  const c = selectedType.value
  const r = violationResult.value
  if (!c || !r) return []
  const own = r.typeCycle[c.id] ?? []
  const owner = r.pkgCycle[c.parent] ?? []
  return [...new Set([...own, ...owner])]
})
const selectedTypeFanin = computed<number>(() => {
  const c = selectedType.value
  const r = violationResult.value
  return c && r ? (r.fanIn.get(c.id) ?? 0) : 0
})
const selectedTypeFanout = computed<number>(() => {
  const c = selectedType.value
  const r = violationResult.value
  return c && r ? (r.fanOut.get(c.id) ?? 0) : 0
})
const selectedTypeFaninTypes = computed<number>(() => {
  const c = selectedType.value
  const r = violationResult.value
  return c && r ? (r.fanInClasses.get(c.id) ?? 0) : 0
})
const selectedTypeFanoutClasses = computed<number>(() => {
  const c = selectedType.value
  const r = violationResult.value
  return c && r ? (r.fanOutClasses.get(c.id) ?? 0) : 0
})
const selectedTypeHighFanin = computed<boolean>(() => {
  const c = selectedType.value
  const r = violationResult.value
  if (!c || !r) return false
  return r.highFanin.has(c.id)
})
const selectedTypeHighFanout = computed<boolean>(() => {
  const c = selectedType.value
  const r = violationResult.value
  if (!c || !r) return false
  return r.highFanout.has(c.id)
})

/** 选中类的成员列表（叶子主框类取 leafData.members，非叶子主框直接类取 directTypeMembers）。 */
const selectedTypeMembers = computed<MemberInfo[]>(() => {
  const c = selectedType.value
  if (!c) return []
  return leafData.value[c.parent]?.members?.[c.id] ?? directTypeMembers.value[c.id] ?? []
})

/** 11.14：L3 源码在新标签页打开（hash 路由 #/l3/{qn}，qn 编码）。 */
function openSourceTab(qn: string): void {
  const url = `${window.location.pathname}#/l3/${encodeURIComponent(qn)}`
  window.open(url, '_blank', 'noopener')
}

/** 11.14：点击具体边 → 新标签页 L3 源码，双节点预览（源类 + 目标类）。 */
function openEdgeSourceTab(it: { source: string; target: string }): void {
  const url = `${window.location.pathname}#/l3/${encodeURIComponent(it.source)}/${encodeURIComponent(it.target)}`
  window.open(url, '_blank', 'noopener')
}

/** 11.15：展开/收起类到类汇聚边的下一层实际边（调用点级）。 */
async function toggleActualEdges(it: EdgeListItem): Promise<void> {
  if (it.expanded) {
    it.expanded = false
    return
  }
  if (!it.actual) {
    it.loading = true
    try {
      it.actual = await store.loadActualEdges(it.source, it.target)
    } finally {
      it.loading = false
    }
  }
  it.expanded = true
}

/** 11.15：点击实际边 → 新标签页 L3 源码，双节点预览并高亮调用点行。
 *  源文件定位到调用点（sourceQn 方法 + line），目标文件定位到目标方法。 */
function openActualEdgeSourceTab(it: EdgeListItem, a: ActualEdge): void {
  const params = new URLSearchParams({ s: it.source, t: it.target, sq: a.sourceQn, tq: a.targetQn })
  // 类型级关系（TYPE_ARG/EXTEND/IMPLEMENT 等）无调用点行号，不传 line（落到类首行）。
  if (a.line > 0) params.set('line', String(a.line))
  const url = `${window.location.pathname}#/l3/${encodeURIComponent(it.source)}/${encodeURIComponent(it.target)}?${params.toString()}`
  window.open(url, '_blank', 'noopener')
}

/** 选中包并关闭类详情/边详情（三类详情面板同定位，需互斥）；传 null 仅关闭包详情。 */
function selectPkg(id: string | null): void {
  selectedType.value = null
  selectedEdge.value = null
  selectedQn.value = id
}

// 单击包（combo）→ 仅选中显示细节（11.4：不再自动下钻加载子节点）。
// 双击（onComboDblClick）或右键-下钻才切换主框加载子节点。
function onComboClick(id: string): void {
  selectPkg(id)
}

// 双击包（combo）→ 下钻成为主框 + 加载子节点（11.4 新增，11.6 多主框 drillTo）。
// 叶子包双击 = 下钻展示其类（就地展开，下钻链终点）；非叶子 = 展示直接子包。
function onComboDblClick(id: string): void {
  drillTo(id)
}

// 右键菜单（决策 D3：下钻/折叠/源码，去扇入扇出与聚焦）。
function onNodeAction(id: string, action: 'drill' | 'collapse' | 'source'): void {
  if (action === 'drill') {
    drillTo(id)
    return
  }
  if (action === 'collapse') {
    // 折叠（目标4）：移除选中点的子节点（含子孙），自身缩小为本体（折叠壳）。
    collapsePkg(id)
    return
  }
  if (action === 'source') {
    // 查看源码 → 进 L3（面包屑 L1 → L3）。
    emit('go-source', id)
  }
}

// 点击叶子节点（无子包的包）→ 仅选中显示细节；进 L2 由面板按钮触发。
// 点击类节点（主框内混合显示，决策 Q4）→ 显示类详情（归属包 + 成员数）。
function onNodeClick(id: string): void {
  const cls = allTypeNodes.value.find((n) => n.id === id)
  if (cls) {
    selectPkg(null)
    selectedEdge.value = null
    selectedType.value = cls
    return
  }
  selectPkg(id)
}

// 双击叶子包（node 单点）→ 下钻就地展开其类（11.4 #55 下钻链终点：主框叶子 → 类直接显示在主框内）。
function onNodeDblClick(id: string): void {
  drillTo(id)
}

function onCloseDetail(): void {
  selectedQn.value = null
  selectedType.value = null
  selectedEdge.value = null
}

// ---- 点击边 → 边详情（目标 9，11.6；11.7 补具体边列表） ----
interface EdgeListItem {
  source: string
  target: string
  type: string
  count: number
  /** 方法级调用明细（类级边/跨包边有 calls）。 */
  calls?: EdgeCallDetail[]
  /** 11.15：实际边（调用点级）列表，仅类到类汇聚边展开后填充。 */
  actual?: ActualEdge[]
  /** 11.15：是否已展开实际边。 */
  expanded?: boolean
  /** 11.15：实际边加载中。 */
  loading?: boolean
}
const selectedEdge = ref<{
  source: string
  target: string
  count: number
  types: string[]
  kind: 'pkg' | 'type'
  /** 该边下的具体边列表（11.7 问题4）。 */
  items: EdgeListItem[]
} | null>(null)
function onEdgeClick(source: string, target: string): void {
  // 从画布边数据（包级汇聚边 / 类间明细边）查找匹配边。
  const hit = allCanvasEdges.value.find((e) => e.source === source && e.target === target)
  // 清包/类详情（同定位互斥）。
  selectedQn.value = null
  selectedType.value = null
  if (!hit) {
    selectedEdge.value = { source, target, count: 1, types: [], kind: 'pkg', items: [] }
    return
  }
  const kind = leafTypeEdges.value.some((e) => e.source === hit.source && e.target === hit.target)
    ? 'type'
    : 'pkg'
  let items: EdgeListItem[] = []
  if (kind === 'type') {
    // 类间明细边：展示两端类 + 方法级调用明细。
    const cls = leafTypeEdges.value.find((e) => e.source === hit.source && e.target === hit.target)
    if (cls) items = [{ source: cls.source, target: cls.target, type: (cls.types ?? [])[0] ?? cls.type, count: cls.count ?? 1, calls: cls.calls }]
  } else {
    // 汇聚边：从类级边上卷出「两端落到本边源/目标节点」的具体类到类边（11.12）。
    // 这样边详情能给出真实明细，而不是预聚合的包级边。
    const ids = visibleAll.value
    const p = payload.value
    const parent = new Map((p?.tree ?? []).map((t) => [t.qn, t.parent]))
    const va = (qn: string): string => {
      let cur = qn
      while (cur && !ids.has(cur)) {
        cur = parent.has(cur)
          ? (parent.get(cur) ?? '')
          : (cur.includes('.') ? cur.slice(0, cur.lastIndexOf('.')) : '')
      }
      return cur
    }
    const acc = new Map<string, EdgeListItem>()
    for (const e of typeEdges.value) {
      if (va(e.source) !== hit.source || va(e.target) !== hit.target) continue
      const key = e.source + '->' + e.target
      const x = acc.get(key)
      if (x) x.count += e.count ?? 1
      else acc.set(key, { source: e.source, target: e.target, type: (e.types ?? [])[0] ?? 'DEPEND', count: e.count ?? 1 })
    }
    items = [...acc.values()].sort((a, b) => b.count - a.count)
  }
  selectedEdge.value = {
    source,
    target,
    count: hit.count ?? 1,
    types: hit.types ?? [hit.type],
    kind,
    items,
  }
  activeEdgeTab.value = '' // 重置为「全部」
  edgeGroupMode.value = 'type' // 默认按 REL 类型
  edgeEndpointDir.value = 'out'
}

/** 边详情端点点击：关闭边详情，打开对应节点详情（决策 A：详情只展示+跳转，端点可溯源）。 */
function onEdgeEndpointClick(qn: string): void {
  if (!qn) return
  selectedEdge.value = null
  const cls = allTypeNodes.value.find((n) => n.id === qn)
  if (cls) {
    selectedQn.value = null
    selectedType.value = cls
    return
  }
  selectedQn.value = qn
  selectedType.value = null
}

// ---- 边详情三档汇聚（11.12）：REL 类型 / 端点(扇出源·扇入目标) / 类到类明细 ----
const activeEdgeTab = ref('')
/** 汇聚维度（11.14）：type=按 REL 类型；endpoint=按扇出源类/扇入目标类。
 *  移除「类到类」档——明细列表本身即类到类边，无需单独维度。 */
const edgeGroupMode = ref<'type' | 'endpoint'>('type')
/** 端点汇聚方向：out=按扇出源类（源→目标）；in=按扇入目标类。 */
const edgeEndpointDir = ref<'out' | 'in'>('out')

/** 边类型分组：按 items[].type 聚合，tab 展示各类型 + 数量。 */
const edgeTypeGroups = computed<{ type: string; items: EdgeListItem[] }[]>(() => {
  const items = selectedEdge.value?.items ?? []
  const order: string[] = []
  const map = new Map<string, EdgeListItem[]>()
  for (const it of items) {
    const t = it.type || 'DEPEND'
    const arr = map.get(t)
    if (arr) arr.push(it)
    else {
      map.set(t, [it])
      order.push(t)
    }
  }
  return order.map((t) => ({ type: t, items: map.get(t)! }))
})

/** 端点分组：按扇出源类（out）或扇入目标类（in）聚合，展示端点 + 条数。 */
const edgeEndpointGroups = computed<{ endpoint: string; count: number; items: EdgeListItem[] }[]>(() => {
  const items = selectedEdge.value?.items ?? []
  const dir = edgeEndpointDir.value
  const map = new Map<string, { count: number; items: EdgeListItem[] }>()
  for (const it of items) {
    const ep = dir === 'out' ? it.source : it.target
    const hit = map.get(ep)
    if (hit) {
      hit.count += it.count
      hit.items.push(it)
    } else {
      map.set(ep, { count: it.count, items: [it] })
    }
  }
  return [...map.entries()]
    .map(([endpoint, v]) => ({ endpoint, count: v.count, items: v.items }))
    .sort((a, b) => b.count - a.count)
})

/** 当前展示的边列表（11.14：统一按全类名排序）：
 *  type 模式按 activeEdgeTab 过滤 REL 类型；endpoint 模式按选中端点过滤。 */
const edgeTabItems = computed<EdgeListItem[]>(() => {
  const items = selectedEdge.value?.items ?? []
  let out = items
  if (edgeGroupMode.value === 'endpoint') {
    const ep = activeEdgeTab.value
    if (ep) {
      const dir = edgeEndpointDir.value
      out = items.filter((it) => (dir === 'out' ? it.source : it.target) === ep)
    }
  } else if (activeEdgeTab.value) {
    out = items.filter((it) => (it.type || 'DEPEND') === activeEdgeTab.value)
  }
  // 按名称排序（源全类名 → 目标全类名）。
  return [...out].sort((a, b) => {
    const ka = `${a.source}->${a.target}`
    const kb = `${b.source}->${b.target}`
    return ka < kb ? -1 : ka > kb ? 1 : 0
  })
})

/** 切换汇聚维度时清空 tab 选中，避免跨维度残留过滤。 */
function setEdgeGroupMode(m: 'type' | 'endpoint'): void {
  edgeGroupMode.value = m
  activeEdgeTab.value = ''
}
function setEdgeEndpointDir(d: 'out' | 'in'): void {
  edgeEndpointDir.value = d
  activeEdgeTab.value = ''
}

/** 选中包详情（11.14）：
 *  - 子包数 = 直接子包数；类规模 = 子树类规模（typeCount）。
 *  - 扇入/扇出按**类级边**统计（不按包级汇聚边），口径 = 离开/进入本包子树的类到类边条数 + 对端包数。
 *  - 违规标记向上汇聚：包自身或子树内存在环/高扇入即标记。 */
const selectedDetail = computed(() => {
  const id = selectedQn.value
  const p = payload.value
  if (!id || !p) return null
  const n = p.nodes.find((x) => x.id === id)
  if (!n) return null
  const sub = subtreeQns(id)
  // 类 QN → 所属包（类级边端点是类，需先归到包再判断是否在本包子树内）。
  const pkgOf = (qn: string): string => {
    let cur = qn
    while (cur && !p.nodes.some((x) => x.id === cur)) {
      cur = cur.includes('.') ? cur.slice(0, cur.lastIndexOf('.')) : ''
    }
    return cur
  }
  let edgeIn = 0
  let edgeOut = 0
  const inPkgs = new Set<string>()
  const outPkgs = new Set<string>()
  // 11.14：改用类级边统计（包级边是预聚合，粒度过粗）。
  for (const e of typeEdges.value) {
    const sp = pkgOf(e.source)
    const tp = pkgOf(e.target)
    if (!sp || !tp) continue
    const sIn = sub.has(sp)
    const tIn = sub.has(tp)
    if (sIn && !tIn) {
      edgeOut += e.count ?? 1
      outPkgs.add(tp)
    } else if (!sIn && tIn) {
      edgeIn += e.count ?? 1
      inPkgs.add(sp)
    }
  }
  // 违规向上汇聚（11.16）：环组按直接所属包投影，高扇入/扇出按类级命中后向上累加。
  const vr = violationResult.value
  const cycle = [...new Set(
    Object.entries(vr?.pkgCycle ?? {})
      .filter(([q]) => sub.has(q))
      .flatMap(([, ids]) => ids),
  )].sort((a, b) => a - b)
  const intra = false
  const highFanin = (vr?.subtreeFaninCount[id] ?? 0) > 0
  const highFanout = (vr?.subtreeFanoutCount[id] ?? 0) > 0
  const faninClasses = vr?.subtreeFaninCount[id] ?? 0
  const fanoutClasses = vr?.subtreeFanoutCount[id] ?? 0
  // 所属模块 = 最近的业务根直接子包。
  let mod = ''
  const rootKids = BUSINESS_ROOT.value ? pkgChildren(BUSINESS_ROOT.value) : []
  let cur = id
  const parent = new Map(p.tree.map((t) => [t.qn, t.parent]))
  while (cur && !rootKids.includes(cur)) cur = parent.get(cur) ?? ''
  if (cur) mod = cur.split('.').pop() ?? cur
  return {
    id,
    label: n.label ?? id.split('.').pop() ?? id,
    kind: n.kind ?? 'PACKAGE',
    depth: n.depth ?? 0,
    /** 直接子包数（11.14）。 */
    children: n.children ?? 0,
    /** 子树类规模（11.14，overview.json 真实类数）。 */
    size: n.typeCount ?? n.size ?? 0,
    parent: n.parent ?? '',
    module: mod,
    cycle,
    intra,
    highFanin,
    highFanout,
    faninClasses,
    fanoutClasses,
    edgeIn,
    edgeOut,
    inPkgs: inPkgs.size,
    outPkgs: outPkgs.size,
    isMain: mainBoxes.value.includes(id),
    isLeaf: (n.children ?? 0) === 0,
    /** 11.14：包节点也可跳 L2 蝴蝶（看辐射影响）；类节点仅类/接口/枚举。 */
    canBlast: true,
    /** 11.14：仅类节点可跳 L3 源码（包节点无源码）。 */
    canSource: n.kind !== 'PACKAGE' && n.kind !== 'UNKNOWN',
  }
})


/** 11.15：hover tooltip 信息（与节点详情弹窗「基本信息」一致）。
 *  包节点复用 selectedDetail 的口径；类节点按类详情口径（类型/成员数/归属包 + 违规向上汇聚）。 */
interface NodeTipInfo {
  label: string
  kind: string
  rows: { k: string; v: string }[]
  tags: string[]
}
const nodeTipInfo = computed<Record<string, NodeTipInfo>>(() => {
  const out: Record<string, NodeTipInfo> = {}
  const p = payload.value
  if (!p) return out
  const vr = violationResult.value
  // 包节点：与详情弹窗一致（类型/层级/子包数/类规模/所属模块/父包 + 扇入扇出 + 违规）。
  for (const n of p.nodes) {
    const id = n.id
    const sub = subtreeQns(id)
    const cycle = Object.keys(vr?.pkgCycle ?? {}).filter((q) => sub.has(q))
    const faninClasses = vr?.subtreeFaninCount[id] ?? 0
    const fanoutClasses = vr?.subtreeFanoutCount[id] ?? 0
    const tags: string[] = []
    if (cycle.length) tags.push(`包间环 #${cycle.join(', #')}`)
    if (faninClasses > 0) tags.push(`高扇入 ${faninClasses} 类`)
    if (fanoutClasses > 0) tags.push(`高扇出 ${fanoutClasses} 类`)
    out[id] = {
      label: n.label ?? id.split('.').pop() ?? id,
      kind: n.kind ?? 'PACKAGE',
      rows: [
        { k: '层级', v: `depth ${n.depth ?? 0}` },
        { k: '子包数', v: String(n.children ?? 0) },
        { k: '类规模', v: String(n.typeCount ?? n.size ?? 0) },
      ],
      tags,
    }
  }
  // 类节点：类型/成员数/归属包 + 违规（发生点在类，环向上取宿主包投影）。
  for (const c of allTypeNodes.value) {
    const tags: string[] = []
    const cyc = [...new Set([...(vr?.typeCycle[c.id] ?? []), ...(vr?.pkgCycle[c.parent] ?? [])])]
    if (cyc.length) tags.push(`包间环 #${cyc.join(', #')}`)
    if (vr?.highFanin.has(c.id)) tags.push(`高扇入 ${vr.fanIn.get(c.id) ?? 0}`)
    if (vr?.highFanout.has(c.id)) tags.push(`高扇出 ${vr.fanOut.get(c.id) ?? 0}`)
    out[c.id] = {
      label: c.label,
      kind: c.kind,
      rows: [
        { k: '成员数', v: String(c.memberCount ?? 0) },
        { k: '归属包', v: c.parent.split('.').pop() ?? c.parent },
      ],
      tags,
    }
  }
  return out
})

// 包路径条（面包屑）：无主框 = 全景；有主框 = 最新主框的祖先链（可点击回退/重激活）。
// 点击祖先链第 index 个包 → 子切父/切祖先（丢弃子孙），该包成为最新主框。
function onFocusBack(index: number): void {
  if (index < 0) {
    // 回全景：清空主框 + 展开集合 + 折叠集合（回到业务根初始视图）。
    mainBoxes.value = []
    expandedPkgs.value = new Set()
    collapsedPkgs.value = new Set()
    return
  }
  const chain = ancestorChain(mainComboQn.value ?? '')
  const target = chain[index]
  if (target) drillTo(target)
}

// 主框折叠按钮（✕，决策 3）：点击主框右上角 ✕ → 折叠该主框（移除其子节点，缩小为本体）。
function onComboCollapse(id: string): void {
  collapsePkg(id)
}

// 环成员集合与高扇入/高扇出集合（11.16 改口径：类级发生点 → 包级投影 → 可见节点）。
//  - 类级环：SCC 成员的直接所属包（pkgCycle）→ 红描边（包间环）
//  - 高扇入/高扇出：类级阈值命中 → 宿主包 → 橙/绿描边
// 包内环不再计算（EnumValidator↔EnumValid / UuidsValidator↔UuidsValid 是合理的注解-校验器双向引用）。
const cycleOf = computed(() => {
  const m: Record<string, number[]> = {}
  const r = violationResult.value
  if (!r) return m
  const ids = visibleAll.value
  for (const id of ids) if (r.pkgCycle[id]) m[id] = r.pkgCycle[id]
  return m
})
// 11.17：包节点按子树汇聚计数标记；**类节点按自身阈值命中标记**（发生点在类）。
// 原实现只查 subtreeFaninCount（按包 QN 建键），导致类节点永远拿不到高扇入/高扇出标记。
const highFaninSet = computed(() => {
  const s = new Set<string>()
  const r = violationResult.value
  if (!r) return s
  const ids = visibleAll.value
  for (const id of ids) {
    if ((r.subtreeFaninCount[id] ?? 0) > 0 || r.highFanin.has(id)) s.add(id)
  }
  return s
})
const highFanoutSet = computed(() => {
  const s = new Set<string>()
  const r = violationResult.value
  if (!r) return s
  const ids = visibleAll.value
  for (const id of ids) {
    if ((r.subtreeFanoutCount[id] ?? 0) > 0 || r.highFanout.has(id)) s.add(id)
  }
  return s
})
/** 某可见包环类型：1=包间环成员（红）。包内环（2/紫）已取消。 */
const cycleKindOf = computed<Record<string, number>>(() => {
  const m: Record<string, number> = {}
  const r = violationResult.value
  if (!r) return m
  const ids = visibleAll.value
  for (const id of ids) if (r.pkgCycle[id]?.length) m[id] = 1
  return m
})
// ---- 违规摘要（11.16：口径改为类级发生点） ----
const violations = computed(() => {
  const r = violationResult.value
  const p = payload.value
  if (!r || !p) return { cycleGroups: 0, cyclePkgs: 0, highFaninPkgs: 0 }
  return {
    cycleGroups: r.cycles.length,
    cyclePkgs: new Set(r.cycles.flatMap((c) => c.pkgMembers)).size,
    highFaninPkgs: new Set([...r.highFanin].map((q) => r.pkgOf[q] ?? directPkg(q)).filter(Boolean)).size,
  }
})
const metrics = computed(() => payload.value?.metrics ?? { pkgCount: 0, typeCount: 0, relCount: 0, cycleGroups: 0 })

// ---- DSM 矩阵（5.4 / 5.11 / 5.25 / 5.26 / 5.55 重构 + 11.x DSM v3 适配） ----
// DSM v3（L1/L2 合并设计 §11）：DSM = 气泡视图的表格式投影，共享 mainBoxes / 详情看板。
//  D1 初次进入 = 业务根一层（行=列=业务根直接子层，一层不展开）
//  D2 从气泡切换 = 主框子层矩阵（行=列=主框直接子层，一层不展开，对称；列可再点开）
//  D3 单击行/列头 = 包详情看板（复用 selectedQn/selectedDetail）
//  D4 右键行/列头 = 右键菜单（下钻/折叠/源码；11.6 去聚焦）
//  D7 单击单元格 = 依赖详情窗；双击单元格 = 源+目标双主框进气泡（11.6）
//  D8 气泡 ↔ DSM 无缝切换：共享 mainBoxes / 详情，状态不丢
const allEdges = computed(() => payload.value?.edges ?? [])
/** DSM 矩阵根集合（DSM v3 D1/D2）：无主框 = 业务根直接子层；有主框 = 主框直接子层。
 *  由 mainBoxes 驱动（共享导航状态），切回气泡状态不丢。
 *  11.7（问题5）：主框是叶子包时，矩阵行列展开其类节点（leafData 类级行列），不再退化为单包。 */
const dsmRootPkgs = computed<{ id: string; label: string; depth: number }[]>(() => {
  const p = payload.value
  if (!p) return []
  const rootQn = mainComboQn.value ?? BUSINESS_ROOT.value
  if (!rootQn) return p.nodes.filter((n) => (n.depth ?? 0) === 0).map((n) => ({ id: n.id, label: n.label ?? n.id.split('.').pop() ?? n.id, depth: n.depth ?? 0 }))
  const kids = p.tree.filter((t) => t.parent === rootQn)
  if (kids.length) return p.nodes.filter((n) => kids.some((k) => k.qn === n.id)).map((n) => ({ id: n.id, label: n.label ?? n.id.split('.').pop() ?? n.id, depth: n.depth ?? 0 }))
  // 主框是叶子包：子层 = 叶子包的类节点（leafData 已加载），类间依赖为 DSM 单元格。
  const ld = leafData.value[rootQn]
  if (ld?.nodes.length) {
    return ld.nodes.map((n) => ({ id: n.id, label: n.label ?? n.id.split('.').pop() ?? n.id, depth: 0 }))
  }
  return [{ id: rootQn, label: rootQn.split('.').pop() ?? rootQn, depth: 0 }]
})

/** 懒加载展开集：行头（源包）展开的包。默认只展开第一个模块包；列头（目标包）默认仅业务根层。 */
const dsmRowExpand = ref<Set<string>>(new Set())
const dsmColExpand = ref<Set<string>>(new Set())

function dsmPkgChildren(qn: string): { id: string; label: string; depth: number }[] {
  const p = payload.value
  if (!p) return []
  return p.tree
    .filter((t) => t.parent === qn)
    .map((t) => ({ id: t.qn, label: t.qn.split('.').pop() ?? t.qn, depth: t.depth ?? 0 }))
}

/** DSM 展开集：主框/业务根变化时重置为空集（D1/D2 默认一层不展开；列可再点开）。 */
function resetDsmExpand(): void {
  dsmRowExpand.value = new Set()
  dsmColExpand.value = new Set()
}

// 主框或 payload 变化 → 重置 DSM 展开（矩阵范围跟随主框，一层不展开）。
watch(
  () => mainComboQn.value,
  () => resetDsmExpand(),
)
watch(
  () => payload.value,
  () => resetDsmExpand(),
)

/** 按某展开集懒加载展开包列表（共用逻辑）。 */
function buildDsmEntries(expand: Set<string>): { id: string; label: string; depth: number }[] {
  const entries: { id: string; label: string; depth: number }[] = []
  const addChildren = (qn: string, depth: number) => {
    for (const c of dsmPkgChildren(qn)) {
      entries.push({ id: c.id, label: c.label, depth })
      if (expand.has(c.id)) addChildren(c.id, depth + 1) // 懒加载：该子包也被展开才递归
    }
  }
  for (const tp of dsmRootPkgs.value) {
    entries.push({ id: tp.id, label: tp.label, depth: 0 })
    if (expand.has(tp.id)) addChildren(tp.id, 1)
  }
  return entries
}

/** 行头列表（DSM v3）：根 = dsmRootPkgs（业务根/主框子层，一层不展开），可懒加载下钻/折叠。 */
const dsmRows = computed(() => {
  const rows = buildDsmEntries(dsmRowExpand.value)
  const sorted = [...rows]
  if (dsmSortBy.value === 'name') sorted.sort((a, b) => a.id.localeCompare(b.id))
  else {
    // 按上卷子树强度（出边总条数）降序：该包整棵子树作为源包的所有出边条数和（与 cellCount 同口径）。
    const strength = new Map<string, number>()
    for (const e of allEdges.value) {
      if (e.source === e.target) continue
      const s = subtreeQns(e.source)
      for (const qn of s) strength.set(qn, (strength.get(qn) ?? 0) + (e.count ?? 1))
    }
    sorted.sort((a, b) => (strength.get(b.id) ?? 0) - (strength.get(a.id) ?? 0))
  }
  return sorted
})

/** 列头列表：根 = dsmRootPkgs（一层），点击列头懒加载展开子包（D2 列可再点开）。 */
const dsmCols = computed(() => {
  const cols = buildDsmEntries(dsmColExpand.value)
  // 列头排序与行头一致（同序便于对照）。
  const sorted = [...cols]
  if (dsmSortBy.value === 'name') sorted.sort((a, b) => a.id.localeCompare(b.id))
  else sorted.sort((a, b) => a.id.localeCompare(b.id))
  return sorted
})

// 色阶 5 档（5.26）：白→浅蓝→蓝→深蓝→红
const DSM_STEPS = [
  { max: 10, color: '#ffffff' },
  { max: 50, color: '#dbeafe' },
  { max: 200, color: '#93c5fd' },
  { max: 1000, color: '#3b82f6' },
  { max: Infinity, color: '#ef4444' },
]
function cellColor(count: number): string {
  const step = DSM_STEPS.find((s) => count <= s.max) ?? DSM_STEPS[DSM_STEPS.length - 1]
  return step.color
}

// DSM 单元格依赖条数：精确边优先，否则上卷子树边求和（业务包层也能显示聚合强度）。
// 5.55 懒加载：子树集合一次性预计算（按 qn 记忆化），cellCount 结果再缓存——矩阵重绘时 O(1) 取缓存，仅展开变化时重算受影响单元格。
let subtreeCache = new Map<string, Set<string>>()
let cellCache = new Map<string, number>()
function invalidateCaches(): void {
  subtreeCache = new Map()
  cellCache = new Map()
}
// 数据变化时清缓存（懒加载下只重算可见部分）；展开状态由 toggleExpand 同步清理。
watch(payload, () => invalidateCaches())

function subtreeQns(qn: string): Set<string> {
  const hit = subtreeCache.get(qn)
  if (hit) return hit
  const p = payload.value
  const out = new Set<string>([qn])
  if (p) {
    const stack = [qn]
    while (stack.length) {
      const cur = stack.pop()!
      for (const t of p.tree) {
        if (t.parent === cur) {
          out.add(t.qn)
          stack.push(t.qn)
        }
      }
    }
  }
  subtreeCache.set(qn, out)
  return out
}
function cellCount(row: string, col: string): number {
  const key = row + '\u0000' + col
  const hit = cellCache.get(key)
  if (hit !== undefined) return hit
  let v: number
  // 11.7（问题5）：类级 DSM 行列（叶子主框）→ 直接查类间明细边。
  if (mainComboQn.value && isLeaf(mainComboQn.value)) {
    v = leafTypeEdges.value.find((x) => x.source === row && x.target === col)?.count ?? 0
  } else {
    const exact = allEdges.value.find((x) => x.source === row && x.target === col)
    if (exact) {
      v = exact.count
    } else {
      // 上卷：row 子树任包 → col 子树任包的边条数求和。
      const rs = subtreeQns(row)
      const cs = subtreeQns(col)
      let sum = 0
      for (const e of allEdges.value) {
        if (rs.has(e.source) && cs.has(e.target)) sum += e.count ?? 1
      }
      v = sum
    }
  }
  cellCache.set(key, v)
  return v
}

function toggleRowExpand(id: string): void {
  const s = new Set(dsmRowExpand.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  dsmRowExpand.value = s
  invalidateCaches() // 展开变化立即清缓存，避免矩阵重绘读到旧值
}
function toggleColExpand(id: string): void {
  const s = new Set(dsmColExpand.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  dsmColExpand.value = s
  invalidateCaches()
}

// ---- DSM v3 交互（设计 §11：D3 单击头详情 / D4 右键头菜单 / D7 单元格依赖详情；11.6 去聚焦改双击双主框） ----
/** 11.7（问题5）：叶子主框 DSM 的类级行列（类节点无子包，不可下钻/折叠）。 */
function isDsmTypeEntry(id: string): boolean {
  const main = mainComboQn.value
  if (!main || !isLeaf(main)) return false
  return leafTypeNodes.value.some((n) => n.id === id)
}

// DSM 右键菜单项与气泡一致（下钻/折叠/源码，去聚焦），折叠仅在主框/业务根矩阵根出现。
// 类级行列（叶子主框 DSM）仅保留「查看源码」：类无子包，下钻/折叠无意义。
function dsmHeaderContextMenu(qn: string): { name: string; value: string }[] {
  if (isDsmTypeEntry(qn)) return [{ name: '查看源码', value: 'source' }]
  const items = [{ name: '下钻', value: 'drill' }]
  // 折叠项：主框根（矩阵根 = 主框直接子层时，折叠该主框移除其子节点）。
  if (qn === mainComboQn.value || mainBoxes.value.includes(qn)) items.push({ name: '折叠', value: 'collapse' })
  items.push({ name: '查看源码', value: 'source' })
  return items
}

/** D4 右键行/列头动作：下钻 = DSM 内重排（矩阵以该包为根收窄）；折叠 = 移除该包子节点；源码。
 *  11.7（问题5）：类级行列仅「查看源码」（菜单已限），drill/collapse 对类无意义。 */
function onDsmHeaderAction(qn: string, action: string): void {
  if (action === 'drill') {
    if (isDsmTypeEntry(qn)) return // 类不是包，不可下钻
    // D1/D2 内重排：把该包下钻为主框，矩阵范围跟随主框收窄（不跳气泡）。
    drillTo(qn)
    return
  }
  if (action === 'collapse') {
    if (isDsmTypeEntry(qn)) return // 类无子包，不可折叠
    // 折叠：移除该包的子节点（含子孙），自身缩小为本体。
    collapsePkg(qn)
    return
  }
  if (action === 'source') {
    emit('go-source', qn)
  }
}

/** D3 单击行/列头 = 包详情看板（与气泡一致）。返回 true 表示已处理（阻止展开切换）。
 *  11.7（问题5）：类级行列（叶子主框 DSM）→ 显示类详情（selectedType）。 */
function onDsmHeaderClick(qn: string): void {
  selectedEdge.value = null
  const cls = allTypeNodes.value.find((n) => n.id === qn)
  if (cls) {
    selectedQn.value = null
    selectedType.value = cls
    return
  }
  selectedQn.value = qn
  selectedType.value = null
}

/** D7 单击单元格 (row,col) = 依赖详情窗（替代进 L2）。 */
const dsmCellDetail = ref<{ row: string; col: string; count: number } | null>(null)
function onDsmCellClick(row: string, col: string): void {
  dsmCellDetail.value = { row, col, count: cellCount(row, col) }
}

/** 双击单元格 (row,col) = 源+目标双主框进气泡（用户拍板：必然独立无父子祖先关系）。
 *  11.7（问题5）：类级行列（叶子主框 DSM）双击无效——类不是包，不能做主框。 */
function onDsmCellDblClick(row: string, col: string): void {
  if (isDsmTypeEntry(row) || isDsmTypeEntry(col)) return
  mode.value = 'bubble'
  selectPkg(row)
  // 双主框：row+col 都下钻为主框（11.9：同步维护 expandedPkgs，增节点语义与气泡 drillTo 一致）。
  const e = new Set(expandedPkgs.value)
  e.add(row)
  e.add(col)
  expandedPkgs.value = e
  const c = new Set(collapsedPkgs.value)
  c.delete(row)
  c.delete(col)
  collapsedPkgs.value = c
  const keep = mainBoxes.value.filter((m) => m !== row && m !== col)
  mainBoxes.value = [...keep, row, col]
  void nextTick(() => canvasRef.value?.focusElement(row))
}

// DSM 右键菜单浮层（D4）：行/列头右键 = 包菜单（去单元格右键聚焦）。
const dsmMenu = ref<{ x: number; y: number; qn: string; items: { name: string; value: string }[] } | null>(null)
function openDsmMenu(e: MouseEvent, qn: string): void {
  dsmMenu.value = { x: e.clientX, y: e.clientY, qn, items: dsmHeaderContextMenu(qn) }
}
function onDsmMenuPick(value: string): void {
  const m = dsmMenu.value
  if (m) onDsmHeaderAction(m.qn, value)
  dsmMenu.value = null
}
</script>

<template>
  <div class="l1">
    <header class="l1-head">
      <div class="l1-title">
        <h2>仓库全景 · L1 顶层鸟瞰</h2>
        <span class="l1-sub">Combo 嵌套气泡 / DSM 矩阵 · 违规 = 循环依赖 + 高扇入</span>
      </div>
      <div class="l1-metrics">
        <div class="mc"><span class="mc-v">{{ metrics.pkgCount }}</span><span class="mc-l">包</span></div>
        <div class="mc"><span class="mc-v">{{ metrics.typeCount }}</span><span class="mc-l">类</span></div>
        <div class="mc"><span class="mc-v">{{ metrics.relCount }}</span><span class="mc-l">关系</span></div>
        <div class="mc"><span class="mc-v warn">{{ metrics.cycleGroups }}</span><span class="mc-l">环</span></div>
      </div>
    </header>

    <!-- 违规摘要行（11.16：发生点=类，阈值可配） -->
    <div class="violation-summary">
      <span class="vs-item">
        <b class="vs-cross">{{ violations.cycleGroups }}</b> 个循环依赖组
      </span>
      <span class="vs-item">
        <b>{{ violations.cyclePkgs }}</b> 个涉环包
      </span>
      <span class="vs-item">
        <b>{{ violations.highFaninPkgs }}</b> 个高扇入包
      </span>
      <span class="vs-item vs-cfg" :title="violationSummaryText">口径：{{ violationSummaryText }}</span>
      <span class="vs-hint">红描边 = 跨包环 · 橙描边 = 高扇入 · 绿描边 = 高扇出（包内环不计算）</span>
    </div>

    <!-- 模式切换（5.3） -->
    <div class="mode-switch">
      <button :class="{ active: mode === 'bubble' }" @click="mode = 'bubble'">◉ 气泡视图</button>
      <button :class="{ active: mode === 'dsm' }" @click="mode = 'dsm'">▦ DSM 矩阵</button>
    </div>

    <div v-if="mode === 'bubble'" class="l1-canvas">
      <!-- 包路径条（11.6 多主框面包屑）：全景 + 最新主框的祖先链（可点击回退/重激活） -->
      <div class="focus-bar">
        <button class="fb-crumb" :class="{ active: !mainBoxes.length }" @click="onFocusBack(-1)">全景</button>
        <template v-for="(q, i) in ancestorChain(mainComboQn ?? '')" :key="q">
          <span class="fb-sep">/</span>
          <button class="fb-crumb" :class="{ active: i === 0 }" :title="q" @click="onFocusBack(i)">
            {{ q.split('.').pop() }}
          </button>
        </template>
        <span v-if="mainBoxes.length" class="fb-hint">· 当前包：{{ mainComboQn }}（{{ mainChildren.length }} 个子包）· 共 {{ mainBoxes.length }} 个主框</span>
        <span v-else class="fb-hint">· {{ mainChildren.length }} 个业务模块 · 双击包下钻</span>
      </div>

      <ProtoGraphCanvas
        ref="canvasRef"
        v-if="payload"
        :nodes="allCanvasNodes"
        :edges="allCanvasEdges"
        :combos="visibleCombos"
        :kind-colors="{
          PACKAGE: '#5B8FF9', CLASS: '#5AD8A6', INTERFACE: '#6DC8EC',
          ENUM: '#F6BD16', ANNOTATION: '#FF9D4D', UNKNOWN: '#A0A0A0',
        }"
        :cycle-of="cycleOf"
        :cycle-kind-of="cycleKindOf"
        :cycle-edge-keys="cycleEdgeKeys"
        :high-fanin-nodes="highFaninSet"
        :high-fanout-nodes="highFanoutSet"
        :main-combo-ids="mainBoxes"
        :collapsible-ids="collapsibleIds"
        :highlight-node-ids="highlightedCycleNodes"
        :highlight-edge-keys="highlightedCycleEdges"
        :node-tip-info="nodeTipInfo"
        @node-click="onNodeClick"
        @node-dblclick="onNodeDblClick"
        @combo-click="onComboClick"
        @combo-dblclick="onComboDblClick"
        @combo-collapse="onComboCollapse"
        @node-action="onNodeAction"
        @edge-click="onEdgeClick"
      />

      <!-- 缺陷概览面板（11.16）：口径设置 + 重算 + 类级高扇入/扇出 + 类级环链路 -->
      <div class="defect-panel" :class="{ collapsed: !showDefectPanel }">
        <div class="df-head" @click="showDefectPanel = !showDefectPanel">
          <span class="df-title">缺陷概览</span>
          <span class="df-sub">基于当前展开 · {{ violationSummaryText }}</span>
          <span class="df-toggle">{{ showDefectPanel ? '−' : '+' }}</span>
        </div>
        <div v-if="showDefectPanel" class="df-body">
          <!-- 口径设置（11.16）：阈值 + 环边类型 + 节点归一化；改动后点「重算」生效 -->
          <div class="df-sec df-cfg">
            <div class="df-cfg-head" @click="showViolationSettings = !showViolationSettings">
              <span class="df-sec-title"><i class="df-dot dot-cfg" />口径设置</span>
              <span class="df-cfg-toggle">{{ showViolationSettings ? '收起' : '展开' }}</span>
            </div>
            <div v-if="showViolationSettings" class="df-cfg-body">
              <div class="df-cfg-row">
                <span class="df-cfg-label">扇入/扇出阈值</span>
                <span class="df-cfg-inline">
                  警告 ≥ <input v-model.number="violationDraft.fanWarn" class="df-num" type="number" min="1" step="10" />
                  严重 ≥ <input v-model.number="violationDraft.fanSevere" class="df-num" type="number" min="1" step="10" />
                </span>
              </div>
              <div class="df-cfg-hint">计数口径 = 引用次数（类级边 count 累加）· 发生点在类，包层只做子树汇聚</div>
              <div class="df-cfg-row df-cfg-col">
                <span class="df-cfg-label">环计算边类型（{{ violationDraft.cycleTypes.length }}/{{ allRelTypes.length }}）</span>
                <span class="df-chips">
                  <button
                    v-for="t in allRelTypes"
                    :key="t"
                    class="df-chip"
                    :class="{ on: violationDraft.cycleTypes.includes(t) }"
                    @click="toggleCycleType(t)"
                  >{{ t }}</button>
                </span>
              </div>
              <div class="df-cfg-hint">环 = 类级 SCC（Tarjan）→ 直接所属包投影；仅跨包组计入</div>
              <div class="df-cfg-row">
                <span class="df-cfg-label">节点归一化</span>
                <label class="df-check">
                  <input v-model="violationDraft.mergeNested" type="checkbox" />
                  嵌套类/内部类归并到宿主类
                </label>
              </div>
              <div class="df-cfg-actions">
                <button class="df-btn primary" @click="applyViolationConfig">重算</button>
                <button class="df-btn" @click="resetViolationDraft">恢复默认</button>
                <span v-if="recomputeHint" class="df-cfg-ok">{{ recomputeHint }}</span>
              </div>
            </div>
          </div>

          <div class="df-sec">
            <span class="df-sec-title">
              <i class="df-dot dot-in" />高扇入
              <em class="df-sec-note">≥{{ violationConfig.fanWarn }} 次 · 严重 ≥{{ violationConfig.fanSevere }}（{{ defectFanList.severeIn }}）</em>
            </span>
            <div v-if="!defectFanList.fanin.length" class="df-empty">当前视图无数据</div>
            <div v-for="r in defectFanList.fanin" :key="'in-' + r.qn" class="df-row" :class="{ 'is-class': r.isType, severe: r.severe }" :title="r.qn" @click="onDefectItemClick(r.qn)">
              <span class="df-name">{{ r.label }}</span>
              <span class="df-val">{{ r.value }} 次 / {{ r.classes }} 类</span>
            </div>
          </div>
          <div class="df-sec">
            <span class="df-sec-title">
              <i class="df-dot dot-out" />高扇出
              <em class="df-sec-note">≥{{ violationConfig.fanWarn }} 次 · 严重 ≥{{ violationConfig.fanSevere }}（{{ defectFanList.severeOut }}）</em>
            </span>
            <div v-if="!defectFanList.fanout.length" class="df-empty">当前视图无数据</div>
            <div v-for="r in defectFanList.fanout" :key="'out-' + r.qn" class="df-row" :class="{ 'is-class': r.isType, severe: r.severe }" :title="r.qn" @click="onDefectItemClick(r.qn)">
              <span class="df-name">{{ r.label }}</span>
              <span class="df-val">{{ r.value }} 次 / {{ r.classes }} 类</span>
            </div>
          </div>
          <div class="df-sec">
            <span class="df-sec-title"><i class="df-dot dot-cycle" />循环依赖链路（类级）</span>
            <div v-if="!defectCyclePaths.length" class="df-empty">当前视图无完整环</div>
            <div v-for="(c, i) in defectCyclePaths" :key="'cyc-' + i" class="df-cycle" @click="onDefectCycleClick(c)">
              <span class="df-cycle-tag tag-cross">环 #{{ c.group }}</span>
              <span class="df-cycle-path">
                <template v-for="(l, j) in c.labels" :key="j">
                  <span class="df-cycle-node">{{ l }}</span>
                  <span v-if="j < c.labels.length - 1" class="df-cycle-arrow">→</span>
                </template>
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧节点细节信息框（决策 7，6.2 重设计：对齐网格 + 分区清晰） -->
      <div v-if="selectedDetail" class="detail-panel">
        <div class="dp-head">
          <span class="dp-title">节点详情</span>
          <button class="dp-close" title="关闭" @click="onCloseDetail">✕</button>
        </div>
        <div class="dp-body">
          <div class="dp-name" :title="selectedDetail.id">{{ selectedDetail.label }}</div>
          <div class="dp-qn" :title="selectedDetail.id">{{ selectedDetail.id }}</div>

          <div class="dp-sec">基本信息</div>
          <div class="dp-table">
            <div class="dp-tr"><span>类型</span><b>{{ selectedDetail.kind }}</b></div>
            <div class="dp-tr"><span>层级</span><b>depth {{ selectedDetail.depth }}</b></div>
            <div class="dp-tr"><span>子包数</span><b>{{ selectedDetail.children }}</b></div>
            <div class="dp-tr"><span>类规模</span><b>{{ selectedDetail.size }}</b></div>
            <div class="dp-tr"><span>所属模块</span><b>{{ selectedDetail.module || '—' }}</b></div>
            <div class="dp-tr" v-if="selectedDetail.parent"><span>父包</span><b class="dp-short" :title="selectedDetail.parent">{{ selectedDetail.parent.split('.').pop() }}</b></div>
          </div>

          <div class="dp-sec">依赖（类级边）</div>
          <div class="dp-table">
            <div class="dp-tr"><span>扇入</span><b>{{ selectedDetail.inPkgs }} 包 / {{ selectedDetail.edgeIn }} 边</b></div>
            <div class="dp-tr"><span>扇出</span><b>{{ selectedDetail.outPkgs }} 包 / {{ selectedDetail.edgeOut }} 边</b></div>
          </div>

          <div class="dp-sec">违规标记（子树向上汇聚）</div>
          <div class="dp-tags">
            <span v-if="selectedDetail.cycle.length" class="dp-tag tag-red">跨包环 #{{ selectedDetail.cycle.join(', #') }}</span>
            <span v-if="selectedDetail.highFanin" class="dp-tag tag-orange">高扇入 {{ selectedDetail.faninClasses }} 类</span>
            <span v-if="selectedDetail.highFanout" class="dp-tag tag-green">高扇出 {{ selectedDetail.fanoutClasses }} 类</span>
            <span v-if="!selectedDetail.cycle.length && !selectedDetail.highFanin && !selectedDetail.highFanout" class="dp-tag tag-none">无违规</span>
          </div>
        </div>
        <div class="dp-foot">
          <!-- 11.14：包节点可跳 L2 蝴蝶；L3 源码仅类节点（新标签页打开）。 -->
          <button v-if="selectedDetail.canBlast" class="dp-btn" @click="emit('go-blast', selectedDetail.id)">L2 蝴蝶</button>
          <button v-if="selectedDetail.canSource" class="dp-btn" @click="openSourceTab(selectedDetail.id)">L3 源码</button>
        </div>
      </div>

      <!-- 类节点详情（决策 Q4：主框内包+类混合显示；#40：2 tab = 类详情 + 成员列表） -->
      <div v-if="selectedType" class="detail-panel">
        <div class="dp-head">
          <span class="dp-title">类详情</span>
          <button class="dp-close" title="关闭" @click="onCloseDetail">✕</button>
        </div>
        <div class="dp-tabs">
          <button class="dp-tab" :class="{ active: typeDetailTab === 'detail' }" @click="typeDetailTab = 'detail'">类详情</button>
          <button class="dp-tab" :class="{ active: typeDetailTab === 'members' }" @click="typeDetailTab = 'members'">成员（{{ selectedTypeMembers.length }}）</button>
        </div>
        <div class="dp-body">
          <div v-if="typeDetailTab === 'detail'">
            <div class="dp-name" :title="selectedType.id">{{ selectedType.label }}</div>
            <div class="dp-qn" :title="selectedType.id">{{ selectedType.id }}</div>
            <div class="dp-sec">基本信息</div>
            <div class="dp-table">
              <div class="dp-tr"><span>类型</span><b>{{ selectedType.kind }}</b></div>
              <div class="dp-tr"><span>成员数</span><b>{{ selectedType.memberCount ?? 0 }}</b></div>
              <div class="dp-tr"><span>归属包</span><b class="dp-short" :title="selectedType.parent">{{ selectedType.parent.split('.').pop() }}</b></div>
              <div v-if="!selectedTypeOnCanvas" class="dp-tr"><span>画布状态</span><b class="dp-offcanvas">未在当前视图</b></div>
            </div>
            <div class="dp-sec">依赖（类级）</div>
            <div class="dp-table">
              <div class="dp-tr"><span>扇入</span><b>{{ selectedTypeFanin }} 次 / {{ selectedTypeFaninTypes }} 类</b></div>
              <div class="dp-tr"><span>扇出</span><b>{{ selectedTypeFanout }} 次 / {{ selectedTypeFanoutClasses }} 类</b></div>
            </div>
            <div class="dp-sec">违规标记（发生点在类）</div>
            <div class="dp-tags">
              <span v-if="selectedTypeCycle.length" class="dp-tag tag-red">跨包环 #{{ selectedTypeCycle.join(', #') }}</span>
              <span v-if="selectedTypeHighFanin" class="dp-tag tag-orange">高扇入 {{ selectedTypeFanin }}</span>
              <span v-if="selectedTypeHighFanout" class="dp-tag tag-green">高扇出 {{ selectedTypeFanout }}</span>
              <span v-if="!selectedTypeCycle.length && !selectedTypeHighFanin && !selectedTypeHighFanout" class="dp-tag tag-none">无违规</span>
            </div>
          </div>
          <div v-else class="member-list">
            <div v-if="!selectedTypeMembers.length" class="ml-empty">无成员数据</div>
            <!-- 11.19：方法行可点击 → 方法级 L2 Blast Radius（字段/构造器不可点）。 -->
            <div
              v-for="m in selectedTypeMembers"
              :key="m.id"
              class="member-row"
              :class="{ clickable: isMethodMember(m) }"
              :title="isMethodMember(m) ? m.id + '（点击查看方法级影响面）' : m.id"
              @click="isMethodMember(m) && openMethodBlast(m.id)"
            >
              <span class="mr-kind" :class="'mk-' + m.kind.toLowerCase()">{{ m.kind }}</span>
              <span class="mr-label">{{ m.label }}</span>
              <span class="mr-sig" v-if="m.signature">{{ m.signature }}</span>
              <span v-if="isMethodMember(m)" class="mr-go">影响面 ›</span>
            </div>
          </div>
        </div>
        <div class="dp-foot">
          <!-- 11.17：类不在当前画布时，显式提供「在画布中定位」（下钻到宿主包），避免点击概览就自动改画布。 -->
          <button v-if="!selectedTypeOnCanvas" class="dp-btn" @click="locateTypeOnCanvas(selectedType.id)">在画布中定位</button>
          <button class="dp-btn" @click="emit('go-blast', selectedType.id)">L2 蝴蝶</button>
          <button class="dp-btn" @click="openSourceTab(selectedType.id)">L3 源码</button>
          <button class="dp-btn" @click="selectedType = null">收起</button>
        </div>
      </div>

      <!-- 边详情（11.14 改造）：加宽 + 类型按钮一行 + 端点下拉框 + 全类名 + 点击具体边跳 L3 -->
      <div v-if="selectedEdge" class="detail-panel edge-detail-panel">
        <div class="dp-head">
          <span class="dp-title">边详情</span>
          <button class="dp-close" title="关闭" @click="selectedEdge = null">✕</button>
        </div>
        <div class="dp-body">
          <div class="dp-sec">依赖</div>
          <div class="dp-table">
            <div class="dp-tr"><span>源</span><b class="dp-short dp-link" :title="selectedEdge.source" @click="onEdgeEndpointClick(selectedEdge.source)">{{ selectedEdge.source.split('.').pop() }}</b></div>
            <div class="dp-tr"><span>目标</span><b class="dp-short dp-link" :title="selectedEdge.target" @click="onEdgeEndpointClick(selectedEdge.target)">{{ selectedEdge.target.split('.').pop() }}</b></div>
            <div class="dp-tr"><span>条数</span><b>{{ selectedEdge.count }}</b></div>
            <div class="dp-tr"><span>类型</span><b>{{ selectedEdge.types.length ? selectedEdge.types.join(', ') : '—' }}</b></div>
          </div>
          <div class="dp-qn" :title="selectedEdge.source + ' → ' + selectedEdge.target">{{ selectedEdge.source }} → {{ selectedEdge.target }}</div>

          <div class="dp-sec">具体边（{{ selectedEdge.items.length }}）</div>
          <div v-if="!selectedEdge.items.length" class="el-empty">无明细边数据</div>
          <template v-else>
            <!-- 11.14 两档汇聚：按类型 / 按端点（移除「类到类」档，明细本身即类到类边） -->
            <div class="edge-tabs edge-mode-tabs">
              <button class="edge-tab" :class="{ active: edgeGroupMode === 'type' }" @click="setEdgeGroupMode('type')">按类型</button>
              <button class="edge-tab" :class="{ active: edgeGroupMode === 'endpoint' }" @click="setEdgeGroupMode('endpoint')">按端点</button>
            </div>
            <!-- 按类型：REL 类型按钮一行展示（不换行） -->
            <div v-if="edgeGroupMode === 'type'" class="edge-tabs edge-tabs-nowrap">
              <button class="edge-tab" :class="{ active: activeEdgeTab === '' }" @click="activeEdgeTab = ''">全部（{{ selectedEdge.items.length }}）</button>
              <button v-for="g in edgeTypeGroups" :key="g.type" class="edge-tab" :class="{ active: activeEdgeTab === g.type }" @click="activeEdgeTab = g.type">{{ g.type }}（{{ g.items.length }}）</button>
            </div>
            <!-- 按端点：方向切换 + 端点下拉框（11.14 端点列表改下拉，避免大量 tab 溢出） -->
            <template v-if="edgeGroupMode === 'endpoint'">
              <div class="edge-tabs edge-dir-tabs">
                <button class="edge-tab" :class="{ active: edgeEndpointDir === 'out' }" @click="setEdgeEndpointDir('out')">按扇出源类</button>
                <button class="edge-tab" :class="{ active: edgeEndpointDir === 'in' }" @click="setEdgeEndpointDir('in')">按扇入目标类</button>
                <select class="edge-endpoint-select" :value="activeEdgeTab" @change="activeEdgeTab = ($event.target as HTMLSelectElement).value">
                  <option value="">全部（{{ selectedEdge.items.length }}）</option>
                  <option v-for="g in edgeEndpointGroups" :key="g.endpoint" :value="g.endpoint">{{ g.endpoint }}（{{ g.count }}）</option>
                </select>
              </div>
            </template>
            <div class="edge-list">
              <div v-for="(it, i) in edgeTabItems" :key="i" class="el-item" :title="it.source + ' → ' + it.target">
                <div class="el-line">
                  <button class="el-expand-btn" :class="{ open: it.expanded }" :title="it.expanded ? '收起实际边' : '展开调用点级实际边'" @click.stop="toggleActualEdges(it)">{{ it.expanded ? '▾' : '▸' }}</button>
                  <span class="el-src dp-link" :title="it.source" @click="onEdgeEndpointClick(it.source)">{{ it.source }}</span>
                  <span class="el-arrow">→</span>
                  <span class="el-dst dp-link" :title="it.target" @click="onEdgeEndpointClick(it.target)">{{ it.target }}</span>
                  <span class="el-count">×{{ it.count }}</span>
                  <!-- 11.14/11.15：类到类汇聚边 → 两端类节点源码预览（不定位行） -->
                  <button class="el-source-btn" title="在新标签页预览两端类源码" @click.stop="openEdgeSourceTab(it)">源码</button>
                </div>
                <div class="el-sub">
                  <span class="el-type">{{ it.type }}</span>
                  <span v-if="it.loading" class="el-calls">加载实际边…</span>
                  <span v-else-if="it.actual" class="el-calls">{{ it.actual.length }} 个调用点</span>
                  <span v-else-if="it.calls?.length" class="el-calls">调用 {{ it.calls.length }} 处</span>
                </div>
                <!-- 11.15：展开的调用点级实际边（仅实际边可跳 L3 并高亮调用点行） -->
                <div v-if="it.expanded" class="el-actual-list">
                  <div v-if="!it.actual?.length" class="el-actual-empty">无调用点明细</div>
                  <div v-for="(a, j) in it.actual" :key="j" class="el-actual" :title="a.sourceQn + ' → ' + a.targetQn">
                    <div class="el-actual-line">
                      <span class="el-actual-src">{{ a.sourceQn.split('.').pop() }}</span>
                      <span class="el-arrow">→</span>
                      <span class="el-actual-dst">{{ a.targetQn.split('.').pop() }}</span>
                      <span class="el-actual-count">×{{ a.count }}</span>
                      <button class="el-source-btn" title="在新标签页预览源码并高亮调用点" @click.stop="openActualEdgeSourceTab(it, a)">源码</button>
                    </div>
                    <div class="el-actual-sub">
                      <span class="el-type">{{ a.type }}</span>
                      <span class="el-actual-loc" :title="a.file">{{ a.file.split('/').pop() }}<template v-if="a.line > 0">:{{ a.line }}</template></span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- DSM 矩阵（DSM v3：气泡视图表格式投影，共享 mainBoxes/详情看板，设计 §11；11.6 去聚焦改双击双主框） -->
    <div v-else class="dsm-wrap">
      <div class="dsm-toolbar">
        <!-- 当前包说明（与气泡视图 focus-bar 一致：当前主框包路径 + 矩阵范围） -->
        <div class="dsm-crumb">
          <button class="fb-crumb" :class="{ active: !mainBoxes.length }" @click="onFocusBack(-1)">全景</button>
          <template v-for="(q, i) in ancestorChain(mainComboQn ?? '')" :key="'dsm-' + q">
            <span class="fb-sep">/</span>
            <button class="fb-crumb" :class="{ active: i === 0 }" :title="q" @click="onFocusBack(i)">
              {{ q.split('.').pop() }}
            </button>
          </template>
          <span class="fb-hint">· 当前：{{ mainComboQn ? mainComboQn.split('.').pop() + ' 子层（' + dsmRootPkgs.length + ' 行）' : '业务根一层（' + dsmRootPkgs.length + ' 行）' }}</span>
        </div>
        <span class="dsm-hint">单击行/列头 = 详情看板 · 右键行/列头 = 下钻/折叠/源码 · 单击单元格 = 依赖明细 · 双击单元格 = 源+目标双主框进气泡</span>
      </div>

      <!-- 色阶图例（5.55） -->
      <div class="dsm-legend">
        <span class="dl-label">依赖条数</span>
        <span v-for="s in DSM_STEPS" :key="s.max" class="dl-item">
          <i class="dl-swatch" :style="{ background: s.color, border: '1px solid #e5e6eb' }" />
          {{ s.max === Infinity ? '>1000' : '≤' + s.max }}
        </span>
        <span class="dl-hint">颜色越深 / 越红 = 依赖越强</span>
      </div>

      <div class="dsm-scroll">
        <table class="dsm">
          <thead>
            <tr>
              <th class="rowhead">源 \ 目标</th>
              <th
                v-for="c in dsmCols"
                :key="c.id"
                class="colhead"
                :class="'depth-' + Math.min(c.depth, 3)"
                :title="c.id"
                @click="onDsmHeaderClick(c.id)"
                @contextmenu.prevent="openDsmMenu($event, c.id)"
              >
                <span class="hd-arrow" :class="{ open: dsmColExpand.has(c.id) }" @click.stop="toggleColExpand(c.id)">{{ dsmColExpand.has(c.id) ? '▾' : '▸' }}</span>{{ c.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in dsmRows" :key="r.id">
              <td
                class="rowhead"
                :class="'depth-' + Math.min(r.depth, 3)"
                :title="r.id"
                @click="onDsmHeaderClick(r.id)"
                @contextmenu.prevent="openDsmMenu($event, r.id)"
              >
                <span class="hd-arrow" :class="{ open: dsmRowExpand.has(r.id) }" @click.stop="toggleRowExpand(r.id)">{{ dsmRowExpand.has(r.id) ? '▾' : '▸' }}</span>{{ r.label }}
              </td>
              <td
                v-for="c in dsmCols"
                :key="c.id"
                class="cell"
                :class="{ 'cell-strong': cellCount(r.id, c.id) > 1000 }"
                :style="{ background: cellColor(cellCount(r.id, c.id)) }"
                :title="`${r.id} → ${c.id}: ${cellCount(r.id, c.id)}（双击 = 源+目标双主框）`"
                @click="onDsmCellClick(r.id, c.id)"
                @dblclick="onDsmCellDblClick(r.id, c.id)"
              >
                {{ cellCount(r.id, c.id) || '' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- DSM 右键菜单浮层（D4：下钻/折叠/源码，去聚焦） -->
      <div v-if="dsmMenu" class="dsm-menu-mask" @click="dsmMenu = null" @contextmenu.prevent="dsmMenu = null" />
      <div
        v-if="dsmMenu"
        class="dsm-menu"
        :style="{ left: dsmMenu.x + 'px', top: dsmMenu.y + 'px' }"
        @click.stop
      >
        <button v-for="it in dsmMenu.items" :key="it.value" class="dm-item" @click="onDsmMenuPick(it.value)">
          {{ it.name }}
        </button>
      </div>

      <!-- 单元格依赖详情窗（D7：替代进 L2） -->
      <div v-if="dsmCellDetail" class="dsm-cell-detail" @click.stop>
        <div class="dc-head">
          <span class="dc-title">依赖详情</span>
          <button class="dc-close" @click="dsmCellDetail = null">✕</button>
        </div>
        <div class="dc-body">
          <div class="dc-row"><span>源</span><b :title="dsmCellDetail.row">{{ dsmCellDetail.row }}</b></div>
          <div class="dc-row"><span>目标</span><b :title="dsmCellDetail.col">{{ dsmCellDetail.col }}</b></div>
          <div class="dc-row"><span>条数</span><b>{{ dsmCellDetail.count }}</b></div>
        </div>
        <div class="dc-foot">
          <button class="dc-btn" @click="onDsmCellDblClick(dsmCellDetail.row, dsmCellDetail.col)">源+目标双主框</button>
        </div>
      </div>
    </div>

    <p class="l1-hint">色块 = 业务模块（同色相 = 同模块） · 双击/右键-下钻展开子包 · 右键-折叠移除子包（自身保留） · 单击包/边显示详情（L2 蝴蝶 / L3 源码跳转） · 包路径条可逐级回退 · 节点右上角徽标 = 类型 · 边上 ×N = 依赖条数</p>
  </div>
</template>

<style scoped>
.l1 {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.l1-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px 0;
}
.l1-title h2 {
  margin: 0;
  font-size: 18px;
}
.l1-sub {
  color: #86909c;
  font-size: 12px;
}
.l1-metrics {
  display: flex;
  gap: 12px;
}
.mc {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding: 8px 14px;
  background: #f5f7fa;
  border-radius: 8px;
}
.mc-v {
  font-size: 20px;
  font-weight: 600;
  color: #1f2329;
}
.mc-v.warn {
  color: #e8684a;
}
.mc-l {
  font-size: 12px;
  color: #86909c;
}
.violation-summary {
  display: flex;
  align-items: center;
  gap: 16px;
  margin: 10px 20px 0;
  padding: 8px 12px;
  background: #fff7e6;
  border: 1px solid #ffe7ba;
  border-radius: 8px;
  font-size: 13px;
  color: #4e5969;
}
.vs-item b {
  color: #e8684a;
  font-size: 15px;
}
.vs-item .vs-cross {
  color: #e8684a;
}
.vs-item .vs-intra {
  color: #9270CA;
}
.vs-hint {
  margin-left: auto;
  color: #86909c;
  font-size: 12px;
}
.mode-switch {
  display: flex;
  gap: 6px;
  margin: 10px 20px 0;
}
.mode-switch button {
  padding: 5px 14px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
  color: #4e5969;
}
.mode-switch button.active {
  background: #e8f3ff;
  border-color: #165dff;
  color: #165dff;
}
.l1-canvas {
  position: relative;
  flex: 1;
  min-height: 0;
  margin: 12px 20px;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
  border: 1px solid #e5e6eb;
}

/* 包路径条（聚焦下钻面包屑） */
.focus-bar {
  position: absolute;
  top: 10px;
  left: 12px;
  z-index: 12;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  font-size: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}
.fb-crumb {
  border: none;
  background: transparent;
  color: #4e5969;
  cursor: pointer;
  font-size: 12px;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fb-crumb:hover {
  color: #165dff;
}
.fb-crumb.active {
  color: #165dff;
  font-weight: 600;
}
.fb-sep {
  color: #c0c4cc;
}
.fb-hint {
  margin-left: 8px;
  color: #86909c;
  font-size: 11px;
}

/* 缺陷概览面板（11.13） */
.defect-panel {
  position: absolute;
  top: 52px;
  left: 12px;
  z-index: 12;
  width: 250px;
  max-height: calc(100% - 76px);
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.96);
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.08);
  font-size: 11px;
  overflow: hidden;
}
.df-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  cursor: pointer;
  border-bottom: 1px solid #eef0f3;
  user-select: none;
}
.df-title {
  font-weight: 600;
  font-size: 12px;
  color: #1f2329;
}
.df-sub {
  color: #86909c;
  font-size: 10px;
}
.df-toggle {
  margin-left: auto;
  color: #86909c;
  font-size: 14px;
  line-height: 1;
}
.df-body {
  padding: 6px 10px 10px;
  overflow-y: auto;
}
.df-sec {
  margin-top: 8px;
}
.df-sec:first-child {
  margin-top: 2px;
}
.df-sec-title {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-weight: 600;
  color: #4e5969;
}
.df-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  display: inline-block;
}
.dot-in { background: #F77234; }
.dot-out { background: #2f9e6e; }
.dot-cycle { background: #e8684a; }
.df-empty {
  margin-top: 4px;
  color: #a0a8b3;
}
.df-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 3px;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
}
.df-row:hover {
  background: #f2f5fa;
}
.df-row.is-class .df-name {
  color: #2f9e6e;
}
.df-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #1f2329;
}
.df-val {
  color: #86909c;
  font-variant-numeric: tabular-nums;
}
.df-cycle {
  margin-top: 5px;
  padding: 4px 5px;
  border-radius: 4px;
  cursor: pointer;
  line-height: 1.5;
}
.df-cycle:hover {
  background: #f2f5fa;
}
.df-cycle-tag {
  display: inline-block;
  margin-right: 5px;
  padding: 0 5px;
  border-radius: 8px;
  font-size: 10px;
}
.tag-cross {
  color: #e8684a;
  background: #fdeceb;
}
.tag-intra {
  color: #9270CA;
  background: #f1ebfa;
}
.df-cycle-path {
  color: #4e5969;
}
.df-cycle-node {
  color: #1f2329;
}
.df-cycle-arrow {
  margin: 0 3px;
  color: #c0c4cc;
}

/* 口径设置（11.16）：阈值 + 环边类型 + 节点归一化 + 重算按钮 */
.df-sec-note {
  margin-left: 4px;
  font-style: normal;
  font-weight: 400;
  color: #a0a8b3;
  font-size: 10px;
}
.df-row.severe .df-val {
  color: #e8684a;
  font-weight: 600;
}
.df-cfg {
  border-bottom: 1px solid #eef1f4;
  padding-bottom: 6px;
}
.df-cfg-head {
  display: flex;
  align-items: center;
  cursor: pointer;
}
.df-cfg-toggle {
  margin-left: auto;
  color: #165dff;
  font-size: 10px;
}
.dot-cfg { background: #86909c; }
.df-cfg-body {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.df-cfg-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.df-cfg-row.df-cfg-col {
  align-items: flex-start;
  flex-direction: column;
  gap: 4px;
}
.df-cfg-label {
  color: #4e5969;
  flex-shrink: 0;
}
.df-cfg-inline {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #4e5969;
}
.df-num {
  width: 58px;
  padding: 2px 4px;
  border: 1px solid #d5dae0;
  border-radius: 4px;
  font-size: 11px;
  font-family: inherit;
}
.df-cfg-hint {
  color: #a0a8b3;
  font-size: 10px;
  line-height: 1.5;
}
.df-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.df-chip {
  padding: 1px 6px;
  border: 1px solid #d5dae0;
  border-radius: 8px;
  background: #fff;
  color: #86909c;
  font-size: 10px;
  cursor: pointer;
}
.df-chip.on {
  border-color: #165dff;
  background: #e8f3ff;
  color: #165dff;
}
.df-check {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #4e5969;
  cursor: pointer;
}
.df-cfg-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.df-btn {
  padding: 3px 10px;
  border: 1px solid #d5dae0;
  border-radius: 5px;
  background: #fff;
  color: #4e5969;
  font-size: 11px;
  cursor: pointer;
}
.df-btn.primary {
  border-color: #165dff;
  background: #165dff;
  color: #fff;
}
.df-cfg-ok {
  color: #2f9e6e;
  font-size: 10px;
}
.vs-cfg {
  color: #86909c;
  font-size: 12px;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dsm-wrap {
  flex: 1;
  min-height: 0;
  margin: 12px 20px;
  display: flex;
  flex-direction: column;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  overflow: hidden;
}
.dsm-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid #e5e6eb;
  font-size: 13px;
}
.dsm-sort-hint {
  color: #165dff;
  font-size: 12px;
}
.dsm-hint {
  color: #86909c;
  font-size: 12px;
}
/* DSM 当前包说明（#46：与气泡 focus-bar 一致的面包屑条） */
.dsm-crumb {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: #eef4ff;
  border: 1px solid #91caff;
  border-radius: 8px;
  font-size: 12px;
  flex-shrink: 0;
}
.dsm-crumb .fb-crumb {
  font-size: 12px;
  color: #4e5969;
}

/* 色阶图例 */
.dsm-legend {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-bottom: 1px solid #e5e6eb;
  background: #fafbfc;
  font-size: 12px;
}
.dl-label {
  color: #4e5969;
  font-weight: 600;
}
.dl-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #86909c;
}
.dl-swatch {
  width: 14px;
  height: 14px;
  border-radius: 3px;
  display: inline-block;
}
.dl-hint {
  margin-left: auto;
  color: #86909c;
}

.dsm-scroll {
  flex: 1;
  overflow: auto;
}
.dsm {
  border-collapse: separate;
  border-spacing: 0;
  font-size: 12px;
}
.dsm th,
.dsm td {
  border: 1px solid #eef0f3;
  min-width: 44px;
  height: 26px;
  text-align: center;
  padding: 0 4px;
}
.dsm .rowhead {
  position: sticky;
  left: 0;
  z-index: 2;
  background: #f5f7fa;
  font-weight: 600;
  text-align: left;
  white-space: nowrap;
  cursor: pointer;
  padding-left: 6px;
}
.dsm .colhead {
  position: sticky;
  top: 0;
  z-index: 1;
  background: #f5f7fa;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  padding: 0 6px;
}
.dsm thead th {
  border-top: none;
}
/* 层级缩进：depth 越大颜色越浅、缩进越大 */
.dsm .depth-1 {
  color: #4e5969;
  font-weight: 500;
}
.dsm .depth-2 {
  color: #86909c;
  font-weight: 400;
  padding-left: 14px;
}
.dsm .depth-3 {
  color: #a0a8b3;
  font-weight: 400;
  padding-left: 22px;
}
.hd-arrow {
  display: inline-block;
  width: 12px;
  color: #86909c;
  font-size: 10px;
}
.hd-arrow.open {
  color: #165dff;
}
.dsm .cell {
  cursor: pointer;
  transition: outline 0.1s;
}
.dsm .cell:hover {
  outline: 2px solid #165dff;
  outline-offset: -2px;
}
.dsm .cell-strong {
  color: #fff;
}

/* DSM 右键菜单浮层（D4） */
.dsm-menu-mask {
  position: fixed;
  inset: 0;
  z-index: 999;
}
.dsm-menu {
  position: fixed;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  min-width: 120px;
  padding: 4px;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}
.dm-item {
  padding: 6px 12px;
  border: none;
  background: transparent;
  text-align: left;
  cursor: pointer;
  font-size: 12px;
  color: #1f2329;
  border-radius: 4px;
}
.dm-item:hover {
  background: #e8f3ff;
  color: #165dff;
}

/* 单元格依赖详情窗（D7） */
.dsm-cell-detail {
  position: absolute;
  right: 16px;
  top: 16px;
  z-index: 20;
  width: 300px;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  font-size: 12px;
}
.dc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid #eef0f3;
  background: #f7f8fa;
  border-radius: 10px 10px 0 0;
}
.dc-title {
  font-weight: 600;
  color: #4e5969;
}
.dc-close {
  border: none;
  background: transparent;
  cursor: pointer;
  color: #86909c;
}
.dc-body {
  padding: 10px 12px;
}
.dc-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
.dc-row span {
  width: 40px;
  flex-shrink: 0;
  color: #86909c;
}
.dc-row b {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-foot {
  padding: 8px 12px;
  border-top: 1px solid #eef0f3;
}
.dc-btn {
  width: 100%;
  padding: 6px;
  border: 1px solid #165dff;
  border-radius: 6px;
  background: #165dff;
  color: #fff;
  cursor: pointer;
  font-size: 12px;
}
.l1-hint {
  margin: 0 20px 12px;
  color: #86909c;
  font-size: 12px;
}

/* 右侧节点细节信息框（决策 7） */
.detail-panel {
  position: absolute;
  top: 56px;
  right: 12px;
  z-index: 13;
  width: 260px;
  max-height: calc(100% - 80px);
  overflow-y: auto;
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
  font-size: 12px;
  color: #1f2329;
}
.dp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid #eef0f3;
  background: #f7f8fa;
  border-radius: 10px 10px 0 0;
}
.dp-title {
  font-weight: 600;
  color: #4e5969;
}
.dp-close {
  border: none;
  background: transparent;
  cursor: pointer;
  color: #86909c;
  font-size: 13px;
  padding: 2px 4px;
}
.dp-close:hover {
  color: #e8684a;
}
/* 类详情看板 2 tab（#40） */
.dp-tabs {
  display: flex;
  border-bottom: 1px solid #eef0f3;
}
.dp-tab {
  flex: 1;
  padding: 7px 0;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  color: #86909c;
  border-bottom: 2px solid transparent;
}
.dp-tab.active {
  color: #165dff;
  font-weight: 600;
  border-bottom-color: #165dff;
}
.dp-body {
  padding: 10px 12px;
}
.dp-name {
  font-size: 14px;
  font-weight: 600;
  color: #1f2329;
}
.dp-qn {
  margin-top: 2px;
  color: #86909c;
  font-size: 11px;
  word-break: break-all;
  line-height: 1.4;
}
.dp-sec {
  margin-top: 10px;
  font-weight: 600;
  color: #4e5969;
  font-size: 12px;
  border-left: 3px solid #165dff;
  padding-left: 6px;
}
/* 边详情具体边列表（11.7 问题4） */
/* 11.8（问题2）：边详情按类型分 tab */
.edge-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.edge-tab {
  padding: 2px 10px;
  font-size: 11px;
  line-height: 18px;
  border: 1px solid #d5dae0;
  border-radius: 10px;
  background: #fff;
  color: #4e5969;
  cursor: pointer;
}
.edge-tab.active {
  background: #165dff;
  border-color: #165dff;
  color: #fff;
}
/* 11.14：边详情加宽 + 类型按钮一行 + 端点下拉框 */
.edge-detail-panel {
  width: 560px;
  max-width: calc(100% - 40px);
}
.edge-mode-tabs {
  margin-top: 4px;
}
.edge-mode-tabs .edge-tab {
  font-weight: 600;
}
.edge-tabs-nowrap {
  flex-wrap: nowrap;
  overflow-x: auto;
}
.edge-tabs-nowrap .edge-tab {
  flex-shrink: 0;
}
.edge-dir-tabs {
  margin-top: 6px;
  flex-wrap: nowrap;
  align-items: center;
}
.edge-endpoint-select {
  flex: 1;
  min-width: 0;
  padding: 2px 6px;
  font-size: 11px;
  line-height: 18px;
  border: 1px solid #d5dae0;
  border-radius: 6px;
  background: #fff;
  color: #4e5969;
}
/* 11.14：具体边「源码」按钮 */
.el-source-btn {
  flex-shrink: 0;
  margin-left: 6px;
  padding: 0 6px;
  font-size: 10px;
  line-height: 16px;
  border: 1px solid #d5dae0;
  border-radius: 8px;
  background: #fff;
  color: #165dff;
  cursor: pointer;
}
.el-source-btn:hover {
  background: #e8f1ff;
  border-color: #165dff;
}
/* 11.14：具体边全类名展示（允许换行，不截断） */
.edge-detail-panel .el-src,
.edge-detail-panel .el-dst {
  word-break: break-all;
}
/* 11.15：类到类汇聚边展开实际边 */
.el-expand-btn {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  background: transparent;
  color: #86909c;
  cursor: pointer;
  font-size: 11px;
  line-height: 1;
}
.el-expand-btn:hover {
  color: #165dff;
}
.el-expand-btn.open {
  color: #165dff;
}
.el-actual-list {
  margin: 4px 0 0 20px;
  padding-left: 8px;
  border-left: 2px solid #eef0f3;
}
.el-actual-empty {
  padding: 4px 0;
  color: #a0a8b3;
  font-size: 11px;
}
.el-actual {
  padding: 3px 0;
}
.el-actual + .el-actual {
  border-top: 1px dashed #f2f3f5;
}
.el-actual-line {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: #4e5969;
}
.el-actual-src,
.el-actual-dst {
  word-break: break-all;
}
.el-actual-count {
  color: #86909c;
  flex-shrink: 0;
}
.el-actual-sub {
  display: flex;
  gap: 8px;
  font-size: 10px;
  color: #a0a8b3;
}
.el-actual-loc {
  font-family: monospace;
}
.edge-list {
  margin-top: 8px;
  border: 1px solid #eef0f3;
  border-radius: 6px;
  overflow: hidden;
}
.el-item {
  padding: 5px 8px;
}
.el-item + .el-item {
  border-top: 1px solid #eef0f3;
}
.el-line {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #1f2329;
}
.el-src,
.el-dst {
  max-width: 90px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 11.14：边详情加宽后，源/目标改为全类名可换行展示 */
.edge-detail-panel .el-src,
.edge-detail-panel .el-dst {
  max-width: none;
  overflow: visible;
  white-space: normal;
  flex: 1;
}

/* 可点击端点（边详情源/目标 → 打开节点详情）：链接样式 */
.dp-link {
  cursor: pointer;
  color: #165dff;
}
.dp-link:hover {
  text-decoration: underline;
}
.el-arrow {
  color: #c0c4cc;
}
.el-count {
  margin-left: auto;
  color: #e8684a;
  font-weight: 600;
}
.el-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
  font-size: 11px;
  color: #86909c;
}
.el-empty {
  margin-top: 8px;
  padding: 8px;
  border: 1px dashed #e5e6eb;
  border-radius: 6px;
  color: #86909c;
  font-size: 12px;
  text-align: center;
}
/* 6.2 对齐网格：固定标签列（60px）+ 值列，逐行对齐，不再两列乱排 */
.dp-table {
  margin-top: 8px;
  border: 1px solid #eef0f3;
  border-radius: 6px;
  overflow: hidden;
}
.dp-tr {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
}
.dp-tr + .dp-tr {
  border-top: 1px solid #eef0f3;
}
.dp-tr span {
  width: 60px;
  flex-shrink: 0;
  color: #86909c;
  font-size: 12px;
}
.dp-tr b {
  flex: 1;
  min-width: 0;
  color: #1f2329;
  font-weight: 500;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: right;
}
.dp-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
/* 11.17：类不在当前画布时的状态提示（详情仍可看，定位需显式点击）。 */
.dp-offcanvas {
  color: #86909c;
  font-weight: 400;
}
.dp-tag {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
}
.tag-red {
  background: #fde2e2;
  color: #e8684a;
}
.tag-purple {
  background: #efe9fa;
  color: #9270CA;
}
.tag-orange {
  background: #fff0e6;
  color: #F77234;
}
.tag-green {
  background: #e6f7ee;
  color: #2f9e6e;
}
.tag-none {
  background: #f2f3f5;
  color: #86909c;
}
/* 成员列表（类详情看板 tab 2，#40） */
.member-list {
  margin-top: 8px;
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid #eef0f3;
  border-radius: 6px;
}
.member-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  font-size: 12px;
}
.member-row + .member-row {
  border-top: 1px solid #eef0f3;
}
.member-row.clickable {
  cursor: pointer;
}
.member-row.clickable:hover {
  background: #f2f5fa;
}
.mr-go {
  margin-left: auto;
  color: #165dff;
  font-size: 10px;
  flex-shrink: 0;
}
.mr-kind {
  flex-shrink: 0;
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 10px;
  color: #fff;
}
.mk-method {
  background: #5B8FF9;
}
.mk-field {
  background: #F6BD16;
}
.mk-constructor {
  background: #9270CA;
}
.mr-label {
  flex-shrink: 0;
  font-weight: 500;
  color: #1f2329;
}
.mr-sig {
  flex: 1;
  min-width: 0;
  color: #86909c;
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ml-empty {
  padding: 14px;
  text-align: center;
  color: #86909c;
}
.dp-foot {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  border-top: 1px solid #eef0f3;
}
.dp-btn {
  flex: 1;
  padding: 6px 10px;
  border: 1px solid #165dff;
  border-radius: 6px;
  background: #165dff;
  color: #fff;
  cursor: pointer;
  font-size: 12px;
}
.dp-btn:hover {
  background: #0f4ed6;
}
</style>
