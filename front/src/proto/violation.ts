// 违规结果的前端模型与映射。
// 发生点固定在**类**（引用次数加权，阈值可配），环为类级 SCC 的跨包投影；
// 命中/环组全部由后端预计算（/violations + /fan-stats），前端只做包级向上汇聚（纯展示层）。
/** 关系类型全集（维度配置未下发时的兜底）。 */
export const ALL_REL_TYPES = [
  'CALL', 'CREATE', 'USE', 'ASSIGN', 'THROW', 'CAST', 'CAPTURE',
  'EXTEND', 'IMPLEMENT', 'PARAMETER', 'TYPE_ARG', 'RETURN', 'ANNOTATION',
] as const

export interface ViolationConfig {
  /** 扇入/扇出警告档（引用次数加权）。 */
  fanWarn: number
  /** 扇入/扇出严重档。 */
  fanSevere: number
  /** 计入环计算的关系类型。 */
  cycleTypes: string[]
  /** 嵌套类/内部类归并到宿主外部类（消除 X.Builder ↔ X 伪环）。 */
  mergeNested: boolean
}

export const DEFAULT_VIOLATION_CONFIG: ViolationConfig = {
  fanWarn: 50,
  fanSevere: 150,
  cycleTypes: [...ALL_REL_TYPES],
  mergeNested: true,
}

/** 嵌套类归并：取到第一个大写段（含）为止，即宿主外部类 QN。 */
export function hostType(qn: string): string {
  const segs = qn.split('.')
  let i = 0
  while (i < segs.length && !/^[A-Z]/.test(segs[i])) i++
  return i >= segs.length ? qn : segs.slice(0, i + 1).join('.')
}

/** 类的直接所属包（宿主类去最后一段）。 */
export function directPkg(qn: string): string {
  const h = hostType(qn)
  const i = h.lastIndexOf('.')
  return i < 0 ? '' : h.slice(0, i)
}

export interface CycleHop {
  source: string
  target: string
  types: string[]
}

export interface CycleGroup {
  id: number
  /** 类级 SCC 成员（归一化后的类 QN）。 */
  typeMembers: string[]
  /** 类成员的直接所属包（不向上合并）。 */
  pkgMembers: string[]
  /** 组内类级边（用于画布环边高亮与链路展示）。 */
  hops: CycleHop[]
}

export interface ViolationResult {
  config: ViolationConfig
  /** 类 QN → 加权扇入（引用次数）。 */
  fanIn: Map<string, number>
  /** 类 QN → 加权扇出。 */
  fanOut: Map<string, number>
  /** 类 QN → 去重依赖类数（扇入）。 */
  fanInClasses: Map<string, number>
  /** 类 QN → 去重依赖类数（扇出）。 */
  fanOutClasses: Map<string, number>
  /** 达到警告档的类。 */
  highFanin: Set<string>
  highFanout: Set<string>
  /** 达到严重档的类。 */
  severeFanin: Set<string>
  severeFanout: Set<string>
  /** 跨包类级环组。 */
  cycles: CycleGroup[]
  /** 类 QN → 环组 ID 列表。 */
  typeCycle: Record<string, number[]>
  /** 直接所属包 QN → 环组 ID 列表。 */
  pkgCycle: Record<string, number[]>
  /** 类 QN → 直接所属包（真实数据取后端 namespace_qn，mock 由 QN 推导）。 */
  pkgOf: Record<string, string>
  /** 包 QN → 直接包含的高扇入类数。 */
  pkgOwnFanin: Record<string, number>
  /** 包 QN → 直接包含的高扇出类数。 */
  pkgOwnFanout: Record<string, number>
  /** 包 QN → 子树（含自身）内环组数。 */
  subtreeCycleCount: Record<string, number>
  /** 包 QN → 子树（含自身）内高扇入类数。 */
  subtreeFaninCount: Record<string, number>
  /** 包 QN → 子树（含自身）内高扇出类数。 */
  subtreeFanoutCount: Record<string, number>
}

/** 后端扇入扇出指标（/fan-stats）。 */
export interface BackendFanStat {
  qn: string
  pkg: string
  fanIn: number
  fanOut: number
  fanInTypes: number
  fanOutTypes: number
  fanInSites: number
  fanOutSites: number
}

/** 后端违规命中项（/violations）。 */
export interface BackendViolationItem {
  qn: string
  pkg: string
  metric: string
  severity: string
  value: number
  typeCount: number
}

/** 后端环组（/violations）。 */
export interface BackendCycleGroup {
  id: number
  typeMembers: string[]
  pkgMembers: string[]
  hops: { source: string; target: string; types: string[]; siteCount: number }[]
}

/**
 * 真实后端口径：直接采用预计算结果（类级发生点 + 宿主类型归一化 + 跨包环），
 * 不再在前端做 QN 字符串切分（docs/development.md §2.4）。
 * 仅「包级向上汇聚」在前端完成（纯展示层，依赖包树）。
 */
export function violationResultFromBackend(
  fanStats: BackendFanStat[],
  violations: { fanin: BackendViolationItem[]; fanout: BackendViolationItem[]; cycles: BackendCycleGroup[] },
  tree: { qn: string; parent: string }[],
  config: ViolationConfig,
): ViolationResult {
  const fanIn = new Map<string, number>()
  const fanOut = new Map<string, number>()
  const fanInClasses = new Map<string, number>()
  const fanOutClasses = new Map<string, number>()
  const pkgOf = new Map<string, string>()
  for (const f of fanStats) {
    fanIn.set(f.qn, f.fanIn)
    fanOut.set(f.qn, f.fanOut)
    fanInClasses.set(f.qn, f.fanInTypes)
    fanOutClasses.set(f.qn, f.fanOutTypes)
    if (f.pkg) pkgOf.set(f.qn, f.pkg)
  }

  const highFanin = new Set<string>()
  const highFanout = new Set<string>()
  const severeFanin = new Set<string>()
  const severeFanout = new Set<string>()
  for (const it of violations.fanin) {
    highFanin.add(it.qn)
    if (it.severity === 'severe') severeFanin.add(it.qn)
    if (it.pkg) pkgOf.set(it.qn, it.pkg)
  }
  for (const it of violations.fanout) {
    highFanout.add(it.qn)
    if (it.severity === 'severe') severeFanout.add(it.qn)
    if (it.pkg) pkgOf.set(it.qn, it.pkg)
  }

  const cycles: CycleGroup[] = violations.cycles.map((c) => ({
    id: c.id,
    typeMembers: c.typeMembers,
    pkgMembers: c.pkgMembers,
    hops: c.hops.map((h) => ({ source: h.source, target: h.target, types: h.types })),
  }))

  const typeCycle: Record<string, number[]> = {}
  const pkgCycle: Record<string, number[]> = {}
  for (const c of cycles) {
    for (const q of c.typeMembers) (typeCycle[q] ??= []).push(c.id)
    for (const p of c.pkgMembers) (pkgCycle[p] ??= []).push(c.id)
  }

  // 包级向上汇聚（展示层）。
  const parent = new Map(tree.map((t) => [t.qn, t.parent]))
  const ancestorsOf = (pkg: string): string[] => {
    const out: string[] = []
    let cur = pkg
    while (cur) {
      out.push(cur)
      cur = parent.get(cur) ?? ''
    }
    return out
  }
  const pkgOwnFanin: Record<string, number> = {}
  const pkgOwnFanout: Record<string, number> = {}
  const subtreeFaninCount: Record<string, number> = {}
  const subtreeFanoutCount: Record<string, number> = {}
  const subtreeCycleCount: Record<string, number> = {}
  const addUp = (rec: Record<string, number>, pkg: string): void => {
    for (const a of ancestorsOf(pkg)) rec[a] = (rec[a] ?? 0) + 1
  }
  for (const q of highFanin) {
    const pkg = pkgOf.get(q) ?? ''
    if (!pkg) continue
    pkgOwnFanin[pkg] = (pkgOwnFanin[pkg] ?? 0) + 1
    addUp(subtreeFaninCount, pkg)
  }
  for (const q of highFanout) {
    const pkg = pkgOf.get(q) ?? ''
    if (!pkg) continue
    pkgOwnFanout[pkg] = (pkgOwnFanout[pkg] ?? 0) + 1
    addUp(subtreeFanoutCount, pkg)
  }
  for (const c of cycles) {
    const seen = new Set<string>()
    for (const pkg of c.pkgMembers) for (const a of ancestorsOf(pkg)) seen.add(a)
    for (const a of seen) subtreeCycleCount[a] = (subtreeCycleCount[a] ?? 0) + 1
  }

  return {
    config,
    fanIn, fanOut, fanInClasses, fanOutClasses,
    highFanin, highFanout, severeFanin, severeFanout,
    cycles, typeCycle, pkgCycle,
    pkgOf: Object.fromEntries(pkgOf),
    pkgOwnFanin, pkgOwnFanout,
    subtreeCycleCount, subtreeFaninCount, subtreeFanoutCount,
  }
}
