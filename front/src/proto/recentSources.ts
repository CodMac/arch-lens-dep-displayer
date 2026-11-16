// 最近打开的源码目录：存 Cookie，供首页快速选择。
export interface RecentSource {
  path: string
  language: string
  /** 已存在的数据源 sid（可直接进入）。 */
  sid?: string
  /** 最近一次使用时间戳。 */
  at: number
}

const COOKIE = 'arch_lens_recent'
const MAX = 8

function readCookie(name: string): string {
  const prefix = `${name}=`
  for (const part of document.cookie.split(';')) {
    const s = part.trim()
    if (s.startsWith(prefix)) return decodeURIComponent(s.slice(prefix.length))
  }
  return ''
}

/** 读取最近目录（按时间倒序，最多 8 条）。 */
export function loadRecent(): RecentSource[] {
  const raw = readCookie(COOKIE)
  if (!raw) return []
  try {
    const list = JSON.parse(raw) as RecentSource[]
    if (!Array.isArray(list)) return []
    return list
      .filter((x) => x && typeof x.path === 'string' && x.path)
      .sort((a, b) => (b.at ?? 0) - (a.at ?? 0))
      .slice(0, MAX)
  } catch {
    return []
  }
}

/** 记录一次使用：同路径去重后置顶。 */
export function rememberRecent(entry: Omit<RecentSource, 'at'>): void {
  const path = entry.path.trim()
  if (!path) return
  const list = loadRecent().filter((x) => x.path !== path)
  list.unshift({ ...entry, path, at: Date.now() })
  const value = encodeURIComponent(JSON.stringify(list.slice(0, MAX)))
  // 一年有效；SameSite=Lax 避免跨站携带。
  document.cookie = `${COOKIE}=${value}; path=/; max-age=31536000; SameSite=Lax`
}
