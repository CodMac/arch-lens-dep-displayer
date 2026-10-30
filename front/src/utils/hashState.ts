// URL 状态工具（D6 需求20）：下钻路径序列化到 location.hash，刷新/分享不丢。
// 多数据源改造后以数据源 sid 定位（而非维度 id）。
// 格式：#/d/{sid}/{mode}/{level}:{qn}/{level}:{qn}/...
//   mode: overview | level
//   qn 经 encodeURIComponent 编码（含 . / # 等字符）。

/** URL 可恢复的视图状态。 */
export interface HashState {
  sid: string
  mode: 'overview' | 'level'
  /** 下钻栈（不含 ROOT），按从顶到下的顺序。 */
  stack: { level: number; qn: string }[]
}

export function encode(state: HashState): string {
  const segs = state.stack.map((s) => `${s.level}:${encodeURIComponent(s.qn)}`)
  return `#/d/${encodeURIComponent(state.sid)}/${state.mode}/${segs.join('/')}`
}

/** 解析 hash；失败返回 null（触发默认视图）。 */
export function decode(hash: string): HashState | null {
  const h = hash.replace(/^#/, '')
  const parts = h.split('/').filter((x) => x !== '')
  // parts: ['d', sid, mode, level:qn, ...]
  if (parts.length < 3 || parts[0] !== 'd') return null
  const sid = decodeURIComponent(parts[1])
  if (!sid) return null
  const mode = parts[2]
  if (mode !== 'overview' && mode !== 'level') return null
  const stack: { level: number; qn: string }[] = []
  for (const p of parts.slice(3)) {
    const i = p.indexOf(':')
    if (i < 0) continue
    const level = Number(p.slice(0, i))
    const qn = decodeURIComponent(p.slice(i + 1))
    if (!Number.isInteger(level) || level < 1 || !qn) return null
    stack.push({ level, qn })
  }
  return { sid, mode, stack }
}
