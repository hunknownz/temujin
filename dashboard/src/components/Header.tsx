import { useState } from 'react'
import { launchRaid } from '../lib/api'
import type { Task } from '../lib/types'

interface Props {
  tasks: Task[]
  status: string
  onLaunched: () => void
}

export function Header({ tasks, status, onLaunched }: Props) {
  const [title, setTitle] = useState('')
  const [launching, setLaunching] = useState(false)

  const active = tasks.filter((t) => !t.archived && t.state !== 'Done' && t.state !== 'Migrate')
  const running = tasks.filter((t) => !t.archived && !['Done', 'Migrate', 'Inbox'].includes(t.state))

  async function handleLaunch() {
    if (!title.trim() || title.trim().length < 6) return
    setLaunching(true)
    try {
      const r = await launchRaid(title.trim())
      if (r.ok) {
        setTitle('')
        onLaunched()
      }
    } finally {
      setLaunching(false)
    }
  }

  const statusClass = status === 'offline' ? 'text-red-400' : 'text-emerald-400'
  const statusLabel = status === 'live-ws' ? 'Live (WS)' : status === 'live' ? 'Live' : status === 'offline' ? 'Offline' : 'Connecting...'

  return (
    <header className="border-b border-white/10 pb-3 mb-4">
      <div className="flex items-center justify-between mb-3">
        <h1 className="text-xl font-bold text-amber-400">
          Temujin <span className="text-gray-500 text-sm font-normal ml-2">Khan's War Room</span>
        </h1>
        <div className="flex items-center gap-2 text-xs">
          <span className={`px-2.5 py-1 rounded-full bg-white/5 ${statusClass}`}>{statusLabel}</span>
          <span className="px-2.5 py-1 rounded-full bg-white/5 text-gray-400">
            {active.length} active{running.length > 0 && ` (${running.length} running)`}
          </span>
        </div>
      </div>
      <div className="flex gap-2">
        <input
          className="flex-1 bg-gray-900 border border-white/10 rounded-lg px-4 py-2.5 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-amber-500/50 transition-colors"
          placeholder="Describe your raid mission (e.g. 调研AI短视频带货的可行性)"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleLaunch()}
        />
        <button
          className="bg-amber-500 text-black font-semibold px-5 py-2.5 rounded-lg text-sm hover:bg-amber-400 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
          disabled={launching || title.trim().length < 6}
          onClick={handleLaunch}
        >
          {launching ? 'Launching...' : 'Launch Raid'}
        </button>
      </div>
    </header>
  )
}
