// Axios 实例：/api 由 vite 代理到中间层（默认 127.0.0.1:8090，可用 VITE_API_TARGET 覆盖）。
import axios from 'axios'

export const client = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

export async function get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
  const res = await client.get<T>(path, { params })
  return res.data
}

/** POST（D6 需求14：写接口）。 */
export async function post<T>(path: string, data?: unknown): Promise<T> {
  const res = await client.post<T>(path, data)
  return res.data
}

/** PUT（D6 需求14：upsert）。 */
export async function put<T>(path: string, data?: unknown): Promise<T> {
  const res = await client.put<T>(path, data)
  return res.data
}

/** DELETE。 */
export async function del<T>(path: string): Promise<T> {
  const res = await client.delete<T>(path)
  return res.data
}
