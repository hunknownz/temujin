import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchLiveStatus } from '../lib/api'
import type { Task } from '../lib/types'

type ConnStatus = 'connecting' | 'live' | 'live-ws' | 'offline'

export function useRaids() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [status, setStatus] = useState<ConnStatus>('connecting')
  const wsRef = useRef<WebSocket | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const refresh = useCallback(async () => {
    try {
      const t = await fetchLiveStatus()
      setTasks(t)
      setStatus((prev) => (prev === 'live-ws' ? 'live-ws' : 'live'))
    } catch {
      setStatus('offline')
    }
  }, [])

  // Polling fallback
  useEffect(() => {
    refresh()
    pollRef.current = setInterval(refresh, 3000)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [refresh])

  // WebSocket
  useEffect(() => {
    let alive = true

    function connect() {
      if (!alive) return
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${proto}//${location.host}/api/ws`)

      ws.onopen = () => {
        setStatus('live-ws')
        if (pollRef.current) {
          clearInterval(pollRef.current)
          pollRef.current = null
        }
      }

      ws.onmessage = () => {
        refresh()
      }

      ws.onclose = () => {
        wsRef.current = null
        if (alive) {
          if (!pollRef.current) pollRef.current = setInterval(refresh, 3000)
          setTimeout(connect, 2000)
        }
      }

      ws.onerror = () => ws.close()
      wsRef.current = ws
    }

    connect()
    return () => {
      alive = false
      wsRef.current?.close()
    }
  }, [refresh])

  return { tasks, status, refresh }
}
