export function timeAgo(iso: string): string {
  if (!iso) return ''
  const diff = Date.now() - new Date(iso).getTime()
  const m = Math.floor(diff / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

export function fmtTime(iso: string): string {
  if (!iso) return ''
  return new Date(iso).toLocaleTimeString('en', {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

export function stateIndex(state: string): number {
  const map: Record<string, number> = {
    Inbox: 0, Intel: 1, Kurultai: 2, March: 3, Charge: 4, Yam: 5, Loot: 6, Done: 7,
    Blocked: 4, Migrate: 7,
  }
  return map[state] ?? 4
}

export function isRunning(state: string): boolean {
  return !['Done', 'Migrate', 'Inbox', 'Loot', 'Blocked'].includes(state)
}

export function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function renderMd(s: string): string {
  return escapeHtml(s)
    .replace(/^(#{1,3}) (.+)$/gm, '<b class="text-amber-400">$2</b>')
    .replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
    .replace(/\n/g, '<br>')
}
