// 预计算层查询接口（docs/architecture.md §6）：原型视图的真实数据源。
// 所有函数接收维度 id（由 protoStore 从 useDimensionStore 取），返回与 mock 层同构的前端类型。
import { get, put } from './client'
import type {
  OverviewPayload, L2Payload, L2Node, DirectType, TypeEdge, ActualEdge, SourceFile, SourceElement,
} from '../proto/types/model'
import type { FanPayload as FanPayloadResp } from '../proto/fanGraph'

// ---- 后端 DTO（internal/api/dto/proto.go） ----

interface OverviewResp {
  metrics: OverviewPayload['metrics']
  violations: OverviewPayload['violations']
  nodes: OverviewPayload['nodes']
  edges: OverviewPayload['edges']
  tree: OverviewPayload['tree']
}

interface TypeEdgeResp {
  source: string
  target: string
  count: number
  types: string[]
}

interface MemberResp {
  id: string
  label: string
  kind: string
  signature: string
}

interface TypeResp {
  id: string
  label: string
  kind: string
  members?: MemberResp[]
}

interface LeafResp {
  pkg: string
  nodes: TypeResp[]
  edges: TypeEdgeResp[]
  members: Record<string, MemberResp[]>
}

interface CallSiteResp {
  source_qn: string
  target_qn: string
  type: string
  path: string
  line: number
  count: number
}

/** 类型扇入扇出指标（违规口径的数值来源）。 */
export interface FanStatResp {
  qn: string
  pkg: string
  fanIn: number
  fanOut: number
  fanInTypes: number
  fanOutTypes: number
  fanInSites: number
  fanOutSites: number
}

/** 违规命中项。 */
export interface ViolationItemResp {
  qn: string
  label: string
  kind: string
  pkg: string
  metric: string
  severity: string
  value: number
  typeCount: number
}

/** 环组内边。 */
export interface CycleHopResp {
  source: string
  target: string
  types: string[]
  siteCount: number
}

/** 环组。 */
export interface CycleGroupResp {
  id: number
  typeMembers: string[]
  pkgMembers: string[]
  hops: CycleHopResp[]
}

/** 违规摘要 + 列表 + 环组（后端唯一事实来源）。 */
export interface ViolationsResp {
  config: ViolationConfigResp
  fanin: ViolationItemResp[]
  fanout: ViolationItemResp[]
  cycles: CycleGroupResp[]
}

/** 违规口径（后端 violation_config）。 */
export interface ViolationConfigResp {
  fanWarn: number
  fanSevere: number
  cycleRelationTypes: string[]
  mergeNested: boolean
  dedupeSameSite: boolean
  updatedAt: string
}

/** 口径更新请求（undefined 字段表示不修改）。 */
export interface ViolationConfigReq {
  fanWarn?: number
  fanSevere?: number
  cycleRelationTypes?: string[]
  mergeNested?: boolean
  dedupeSameSite?: boolean
}

interface SourceFileResp {
  path: string
  content: string
  lineCount: number
  elements: SourceElement[]
}

// ---- 映射 ----

function toTypeEdges(list: TypeEdgeResp[]): TypeEdge[] {
  return (list ?? []).map((e) => ({ source: e.source, target: e.target, count: e.count, types: e.types ?? [] }))
}

function toMembers(list: MemberResp[] | undefined): MemberResp[] {
  return list ?? []
}

function toCallSites(list: CallSiteResp[]): ActualEdge[] {
  return (list ?? []).map((c) => ({
    sourceQn: c.source_qn,
    targetQn: c.target_qn,
    type: c.type,
    file: c.path,
    line: c.line,
    count: c.count,
  }))
}

// ---- 接口 ----

/** 全景（metrics + 违规摘要 + 包节点 + 包级边 + 包树）。 */
export async function fetchProtoOverview(dimId: number): Promise<OverviewPayload> {
  const r = await get<OverviewResp>(`/dimensions/${dimId}/overview`)
  return {
    metrics: r.metrics,
    violations: r.violations,
    nodes: r.nodes ?? [],
    edges: r.edges ?? [],
    tree: r.tree ?? [],
  }
}

/** 全量类型级边（包级边的汇聚源）。 */
export async function fetchTypeEdges(dimId: number): Promise<TypeEdge[]> {
  return toTypeEdges(await get<TypeEdgeResp[]>(`/dimensions/${dimId}/type-edges`))
}

/** 包的直接类型 + 成员。 */
export async function fetchDirectTypes(dimId: number, pkg: string): Promise<DirectType[]> {
  const list = await get<TypeResp[]>(`/dimensions/${dimId}/types`, { pkg })
  return (list ?? []).map((t) => ({
    id: t.id,
    label: t.label,
    kind: t.kind as DirectType['kind'],
    members: toMembers(t.members),
  }))
}

/** 叶子包负载：类节点 + 类间边 + 成员。 */
export async function fetchLeaf(dimId: number, pkg: string): Promise<L2Payload> {
  const r = await get<LeafResp>(`/dimensions/${dimId}/leaf`, { pkg })
  const members = r.members ?? {}
  const nodes: L2Node[] = (r.nodes ?? []).map((t) => ({
    id: t.id,
    label: t.label,
    kind: t.kind as L2Node['kind'],
    inPkg: true,
    memberCount: (members[t.id] ?? []).length,
    size: (members[t.id] ?? []).length,
  }))
  return {
    pkg: r.pkg,
    nodes,
    edges: (r.edges ?? []).map((e) => ({
      source: e.source,
      target: e.target,
      type: (e.types ?? [])[0] ?? '',
      types: e.types ?? [],
      count: e.count,
      calls: [],
    })),
    members,
  }
}

/** 类型对的方法级调用点明细。 */
export async function fetchCalls(dimId: number, source: string, target: string): Promise<ActualEdge[]> {
  return toCallSites(await get<CallSiteResp[]>(`/dimensions/${dimId}/calls`, { source, target }))
}

/** 方法对的调用点明细。 */
export async function fetchMethodCalls(dimId: number, source: string, target: string): Promise<ActualEdge[]> {
  return toCallSites(await get<CallSiteResp[]>(`/dimensions/${dimId}/method-calls`, { source, target }))
}

/** 元素 kind。 */
export async function fetchKind(dimId: number, qn: string): Promise<string> {
  const r = await get<{ qn: string; kind: string }>(`/dimensions/${dimId}/kind`, { qn })
  return r.kind
}

/** 蝴蝶图（服务端双向 BFS；granularity=type|package|method）。 */
export function fetchFan(
  dimId: number,
  qn: string,
  opts: { granularity: string; depth: number; types?: string[]; hideImplDetail?: boolean },
): Promise<FanPayloadResp> {
  return get<FanPayloadResp>(`/dimensions/${dimId}/fan`, {
    qn,
    granularity: opts.granularity,
    depth: opts.depth,
    types: opts.types && opts.types.length ? opts.types.join(',') : undefined,
    hideImplDetail: opts.hideImplDetail ? 'true' : undefined,
  })
}

/** 全部类型的扇入扇出指标。 */
export function fetchFanStats(dimId: number): Promise<FanStatResp[]> {
  return get<FanStatResp[]>(`/dimensions/${dimId}/fan-stats`)
}

/** 违规摘要 + 命中列表 + 环组（按当前口径预计算）。 */
export function fetchViolations(dimId: number): Promise<ViolationsResp> {
  return get<ViolationsResp>(`/dimensions/${dimId}/violations`)
}

/** 违规口径（画布阈值/环类型以它为准）。 */
export function fetchViolationConfig(dimId: number): Promise<ViolationConfigResp> {
  return get<ViolationConfigResp>(`/dimensions/${dimId}/violations/config`)
}

/** 更新违规口径（落库 + 后端重算派生表），返回重算后的违规摘要。 */
export function putViolationConfig(dimId: number, req: ViolationConfigReq): Promise<ViolationsResp> {
  return put<ViolationsResp>(`/dimensions/${dimId}/violations/config`, req)
}

/** 源码文件（内容 + 元素树）。 */
export async function fetchSourceFile(dimId: number, qn: string): Promise<SourceFile> {
  const r = await get<SourceFileResp>(`/dimensions/${dimId}/source`, { qn })
  return {
    path: r.path,
    content: r.content,
    lineCount: r.lineCount,
    elements: (r.elements ?? []) as SourceElement[],
  }
}
