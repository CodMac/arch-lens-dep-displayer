// 原型全局状态：面包屑下钻栈（L1→L4）+ 数据缓存。
// 数据来源统一为真实后端（../api/proto），维度 id 取自 useDimensionStore。
import { ref, computed } from 'vue'
import * as protoApi from '../api/proto'
import { useDimensionStore } from '../stores/dimension'
import type { OverviewPayload, L2Payload, SourceFile, DirectType, TypeEdge, ActualEdge } from './types/model'
import type { FanPayload } from './fanGraph'

// 面包屑层类型（对应 L1→L2→L3 视图；术语统一：L2=影响面/蝴蝶、L3=源码）。
export interface Crumb {
  label: string
  route: 'overview' | 'l2' | 'l3'
  qn?: string
  /** L2 方法级辐射的目标方法（qn 为宿主类）。 */
  method?: string
}

// 数据单例缓存（避免每次视图切换重复请求/解析）。
const cache = {
  overview: undefined as OverviewPayload | undefined,
  l2: new Map<string, L2Payload>(),
  direct: new Map<string, DirectType[]>(),
  typeEdges: undefined as TypeEdge[] | undefined,
  actual: new Map<string, ActualEdge[]>(),
  fan: new Map<string, FanPayload>(),
  source: new Map<string, SourceFile>(),
  typeKind: new Map<string, string>(),
  methodCall: new Map<string, ActualEdge[]>(),
}

/** 缓存所属维度 id（切换维度时整体失效）。 */
let cachedDimId: number | null = null

function clearCache(): void {
  cache.overview = undefined
  cache.l2.clear()
  cache.direct.clear()
  cache.typeEdges = undefined
  cache.actual.clear()
  cache.fan.clear()
  cache.source.clear()
  cache.typeKind.clear()
  cache.methodCall.clear()
}

export function useProtoStore() {
  const crumbs = ref<Crumb[]>([{ label: '仓库全景', route: 'overview' }])
  const dimensionStore = useDimensionStore()

  const currentCrumb = computed(() => crumbs.value[crumbs.value.length - 1])

  /** 当前维度 id：未选择维度时抛错，避免请求打到 /dimensions/null。 */
  function dimId(): number {
    const id = dimensionStore.currentId
    if (id == null) throw new Error('未选择维度：请先激活数据源')
    // 维度切换（换数据源）后缓存必须失效，否则会展示上一个源的数据。
    if (cachedDimId !== id) {
      clearCache()
      cachedDimId = id
    }
    return id
  }

  async function loadOverview(): Promise<OverviewPayload> {
    if (!cache.overview) {
      cache.overview = await protoApi.fetchProtoOverview(dimId())
    }
    return cache.overview!
  }

  /** 单叶子包类数据（L1/L2 合并：叶子包就地展开类）。 */
  async function loadLeafTypes(leaf: string): Promise<L2Payload> {
    if (!cache.l2.has(leaf)) {
      cache.l2.set(leaf, await protoApi.fetchLeaf(dimId(), leaf))
    }
    return cache.l2.get(leaf)!
  }

  /** 非叶子包的直接类（与子包并列展示）。 */
  async function loadDirectTypes(pkg: string): Promise<DirectType[]> {
    if (!cache.direct.has(pkg)) {
      cache.direct.set(pkg, await protoApi.fetchDirectTypes(dimId(), pkg))
    }
    return cache.direct.get(pkg)!
  }

  /** 类级依赖边（包级边的汇聚源）。 */
  async function loadTypeEdges(): Promise<TypeEdge[]> {
    if (!cache.typeEdges) {
      cache.typeEdges = await protoApi.fetchTypeEdges(dimId())
    }
    return cache.typeEdges
  }

  /** 实际边（调用点级）：按源类→目标类取。 */
  async function loadActualEdges(sourceType: string, targetType: string): Promise<ActualEdge[]> {
    const key = `${sourceType}->${targetType}`
    if (!cache.actual.has(key)) {
      cache.actual.set(key, await protoApi.fetchCalls(dimId(), sourceType, targetType))
    }
    return cache.actual.get(key)!
  }

  /** 蝴蝶图（服务端双向 BFS）。 */
  async function loadFan(
    qn: string,
    opts: { granularity: 'type' | 'package' | 'method'; depth: number; types?: string[]; hideImplDetail?: boolean },
  ): Promise<FanPayload> {
    const key = [qn, opts.granularity, opts.depth, (opts.types ?? []).join(','), opts.hideImplDetail ? 1 : 0].join('\u0000')
    if (!cache.fan.has(key)) {
      cache.fan.set(key, await protoApi.fetchFan(dimId(), qn, opts))
    }
    return cache.fan.get(key)!
  }

  async function loadSource(qn: string): Promise<SourceFile> {
    if (!cache.source.has(qn)) {
      cache.source.set(qn, await protoApi.fetchSourceFile(dimId(), qn))
    }
    return cache.source.get(qn)!
  }

  /** 方法对 → 调用点明细（L2 方法级边详情）。 */
  async function loadMethodCallDetails(sourceMethod: string, targetMethod: string): Promise<ActualEdge[]> {
    const key = `${sourceMethod}\u0000${targetMethod}`
    if (!cache.methodCall.has(key)) {
      cache.methodCall.set(key, await protoApi.fetchMethodCalls(dimId(), sourceMethod, targetMethod))
    }
    return cache.methodCall.get(key)!
  }

  /** 类 QN → 元素类型（缺陷概览点击未渲染类时构造类详情用）。 */
  async function loadTypeKind(qn: string): Promise<string> {
    if (!cache.typeKind.has(qn)) {
      cache.typeKind.set(qn, await protoApi.fetchKind(dimId(), qn))
    }
    return cache.typeKind.get(qn)!
  }

  /** 进入某层：同层替换栈顶（重复下钻），异层压入（保留完整下钻路径，支持逐级回退）。 */
  function enter(crumb: Crumb): void {
    const top = crumbs.value[crumbs.value.length - 1]
    if (top && top.route === crumb.route) {
      crumbs.value[crumbs.value.length - 1] = crumb
    } else {
      crumbs.value.push(crumb)
    }
  }

  /** 回退到指定索引（面包屑点击）。 */
  function backTo(index: number): void {
    if (index < 0 || index >= crumbs.value.length) return
    crumbs.value = crumbs.value.slice(0, index + 1)
  }

  /** 重置回全景。 */
  function reset(): void {
    crumbs.value = [{ label: '仓库全景', route: 'overview' }]
  }

  return {
    crumbs, currentCrumb,
    loadOverview, loadLeafTypes, loadDirectTypes, loadTypeEdges, loadActualEdges,
    loadFan, loadSource, loadTypeKind, loadMethodCallDetails,
    enter, backTo, reset,
  }
}
