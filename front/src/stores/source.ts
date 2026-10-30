// 数据源 store：多数据源列表、当前激活源、创建后自动轮询分析进度。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchSources, createSource, fetchSource, activateSource } from '../api/sources'
import type { Source } from '../types'

const POLL_INTERVAL = 500 // ms

export const useSourceStore = defineStore('source', () => {
  const sources = ref<Source[]>([])
  const activeSid = ref<string | null>(null)
  const creating = ref(false)
  const error = ref('')

  // 轮询句柄（单源单轮询）。
  let pollTimer: number | undefined

  const activeSource = computed<Source | undefined>(() =>
    sources.value.find((s) => s.sid === activeSid.value),
  )

  /** 同步源列表与激活标记（以 server 为准）。 */
  async function load(): Promise<void> {
    try {
      sources.value = await fetchSources()
      const act = sources.value.find((s) => s.active)
      activeSid.value = act?.sid ?? null
    } catch (e) {
      error.value = String(e)
    }
  }

  /** 创建数据源并启动后台分析；轮询直到就绪/失败后自动激活。 */
  async function create(path: string, language?: string): Promise<string> {
    creating.value = true
    error.value = ''
    try {
      const res = await createSource(path, language)
      await load() // 刷新列表
      // 已有活跃轮询则停止。
      stopPoll()
      await pollUntilDone(res.sid)
      return res.sid
    } catch (e) {
      error.value = String(e)
      throw e
    } finally {
      creating.value = false
    }
  }

  /** 轮询指定源直到非 analyzing，随后激活。 */
  async function pollUntilDone(sid: string): Promise<void> {
    await new Promise<void>((resolve) => {
      const tick = async (): Promise<void> => {
        let s: Source
        try {
          s = await fetchSource(sid)
        } catch {
          stopPoll()
          resolve()
          return
        }
        // 同步到列表。
        const idx = sources.value.findIndex((x) => x.sid === sid)
        if (idx >= 0) sources.value[idx] = s
        if (s.status !== 'analyzing') {
          stopPoll()
          if (s.status === 'ready') {
            await activate(sid)
          }
          resolve()
          return
        }
        pollTimer = window.setTimeout(() => void tick(), POLL_INTERVAL)
      }
      void tick()
    })
  }

  function stopPoll(): void {
    if (pollTimer !== undefined) {
      window.clearTimeout(pollTimer)
      pollTimer = undefined
    }
  }

  /** 激活指定源（切换后维度接口数据源改变）。 */
  async function activate(sid: string): Promise<void> {
    try {
      await activateSource(sid)
    } catch (e) {
      error.value = String(e)
      return
    }
    activeSid.value = sid
    await load()
  }

  function reset(): void {
    stopPoll()
    sources.value = []
    activeSid.value = null
  }

  return {
    sources, activeSid, creating, error, activeSource,
    load, create, activate, reset,
  }
})
