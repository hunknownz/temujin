import type { Task, TaskDetail } from './types'

const BASE = ''

export async function fetchLiveStatus(): Promise<Task[]> {
  const r = await fetch(`${BASE}/api/live-status`, { cache: 'no-store' })
  if (!r.ok) throw new Error(`${r.status}`)
  const d = await r.json()
  return d.tasks || []
}

export async function fetchRaidDetail(id: string): Promise<TaskDetail | null> {
  const r = await fetch(`${BASE}/api/raid-detail?id=${encodeURIComponent(id)}`)
  if (!r.ok) return null
  const d = await r.json()
  return d.ok ? d.task : null
}

export async function launchRaid(title: string): Promise<{ ok: boolean; taskId?: string; error?: string }> {
  const r = await fetch(`${BASE}/api/launch-raid`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
  return r.json()
}

export async function raidAction(taskId: string, action: string, reason?: string) {
  const r = await fetch(`${BASE}/api/raid-action`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ taskId, action, reason }),
  })
  return r.json()
}
