// 原型 hash 路由：独立于现有 hashState（不依赖数据源/维度激活）。
// 格式：#/sources（选择数据源）/ #/overview / #/l2/{qn}（影响面·蝴蝶）/ #/l3/{qn}（源码）
// qn 经 encodeURIComponent 编码。

export type ProtoRoute =
  | { name: 'sources' }
  | { name: 'overview' }
  // 11.19：method 可选——带 method 时进入方法级 Blast Radius（qn = 宿主类）。
  | { name: 'l2'; qn: string; method?: string }
  // 11.14：L3 源码支持双节点预览（边详情两端类），qn2 可选。
  // 11.15：line 为调用点行号（实际边跳转时高亮）。
  | { name: 'l3'; qn: string; qn2?: string; line?: number; srcQn?: string; dstQn?: string }

export function encodeRoute(r: ProtoRoute): string {
  switch (r.name) {
    case 'sources': return '#/sources'
    case 'overview': return '#/overview'
    case 'l2': {
      const base = `#/l2/${encodeURIComponent(r.qn)}`
      if (!r.method) return base
      return `${base}?m=${encodeURIComponent(r.method)}`
    }
    case 'l3': {
      const base = r.qn2
        ? `#/l3/${encodeURIComponent(r.qn)}/${encodeURIComponent(r.qn2)}`
        : `#/l3/${encodeURIComponent(r.qn)}`
      const q = new URLSearchParams()
      if (r.line) q.set('line', String(r.line))
      if (r.srcQn) q.set('sq', r.srcQn)
      if (r.dstQn) q.set('tq', r.dstQn)
      const qs = q.toString()
      return qs ? `${base}?${qs}` : base
    }
  }
}

export function decodeRoute(hash: string): ProtoRoute {
  const h = hash.replace(/^#/, '')
  const parts = h.split('/').filter((x) => x !== '')
  // 无 hash（首次打开）= 首页选择数据源。
  if (parts.length < 1) return { name: 'sources' }
  switch (parts[0]) {
    case 'sources':
      return { name: 'sources' }
    case 'overview':
      return { name: 'overview' }
    case 'l2':
      if (parts[1]) {
        const qi = h.indexOf('?')
        const q = new URLSearchParams(qi >= 0 ? h.slice(qi + 1) : '')
        const method = q.get('m') ?? undefined
        // 查询串会粘在最后一段上（如 qn?m=xxx），需先剥离。
        const raw = qi >= 0 ? h.slice(0, qi) : h
        const qnPart = raw.split('/').filter((x) => x !== '')[1]
        if (!qnPart) break
        return { name: 'l2', qn: decodeURIComponent(qnPart), method }
      }
      break
    case 'l3':
      if (parts[1]) {
        // 11.15：查询串携带调用点信息（line/sq/tq）。
        const qi = h.indexOf('?')
        const q = new URLSearchParams(qi >= 0 ? h.slice(qi + 1) : '')
        const line = Number(q.get('line')) || undefined
        const srcQn = q.get('sq') ?? undefined
        const dstQn = q.get('tq') ?? undefined
        return parts[2]
          ? { name: 'l3', qn: decodeURIComponent(parts[1]), qn2: decodeURIComponent(parts[2]), line, srcQn, dstQn }
          : { name: 'l3', qn: decodeURIComponent(parts[1]), line, srcQn, dstQn }
      }
      break
  }
  return { name: 'sources' }
}
