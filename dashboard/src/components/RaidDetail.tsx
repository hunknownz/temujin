import { useEffect, useState } from 'react'
import { fetchRaidDetail, raidAction } from '../lib/api'
import { AGENT_LABELS, STATE_COLORS } from '../lib/types'
import type { Task, TaskDetail } from '../lib/types'
import { fmtTime, renderMd } from '../lib/utils'

interface Props {
  task: Task
  onClose: () => void
  onRefresh: () => void
}

export function RaidDetail({ task, onClose, onRefresh }: Props) {
  const [detail, setDetail] = useState<TaskDetail | null>(null)
  const [activeTab, setActiveTab] = useState(0)

  useEffect(() => {
    fetchRaidDetail(task.id).then((d) => {
      if (d) setDetail(d)
    })
  }, [task.id, task.updatedAt])

  const outputs = detail?.agent_outputs || []
  const flows = detail?.flow_log || task.flow_log || []
  const isActive = !['Done', 'Migrate'].includes(task.state)

  async function doAction(action: string, reason?: string) {
    await raidAction(task.id, action, reason)
    onClose()
    onRefresh()
  }

  return (
    <div
      className="fixed inset-0 bg-black/75 flex items-start justify-center z-50 p-10 overflow-y-auto"
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="bg-gray-900 rounded-xl p-6 w-full max-w-3xl relative border border-white/10">
        <button
          className="absolute top-4 right-4 text-gray-500 hover:text-white text-xl"
          onClick={onClose}
        >
          ×
        </button>

        {/* Header */}
        <h2 className="text-lg font-semibold text-amber-400 pr-8">{task.title}</h2>
        <p className="text-sm text-gray-400 mt-1">
          {task.id} ·{' '}
          <span className="font-semibold" style={{ color: STATE_COLORS[task.state] }}>
            {task.state}
          </span>{' '}
          · {task.org}
        </p>
        {task.now && <p className="text-sm text-amber-400/80 mt-1">{task.now}</p>}
        {task.block && <p className="text-sm text-red-400 mt-1">Blocked: {task.block}</p>}

        {/* Actions */}
        <div className="flex gap-2 mt-3">
          {isActive && (
            <button
              className="text-xs px-3 py-1.5 rounded bg-red-500/20 text-red-400 hover:bg-red-500/30 transition-colors"
              onClick={() => doAction('cancel', 'Cancelled by Khan')}
            >
              Retreat
            </button>
          )}
          {task.state === 'Blocked' && (
            <button
              className="text-xs px-3 py-1.5 rounded bg-amber-500/20 text-amber-400 hover:bg-amber-500/30 transition-colors"
              onClick={() => doAction('resume')}
            >
              Resume
            </button>
          )}
          {!isActive && (
            <button
              className="text-xs px-3 py-1.5 rounded bg-white/5 text-gray-400 hover:bg-white/10 transition-colors"
              onClick={() => doAction('archive')}
            >
              Archive
            </button>
          )}
        </div>

        {/* Agent Outputs */}
        {!detail && outputs.length === 0 && (
          <div className="mt-6 text-sm text-gray-500 flex items-center gap-2">
            <span className="inline-block w-3 h-3 border-2 border-gray-600 border-t-amber-400 rounded-full animate-spin" />
            Loading agent reports...
          </div>
        )}
        {outputs.length > 0 && (
          <div className="mt-6">
            <h3 className="text-xs uppercase tracking-wider text-gray-500 border-b border-white/5 pb-1 mb-3">
              Agent Reports ({outputs.length})
            </h3>
            <div className="flex gap-1 flex-wrap mb-3">
              {outputs.map((o, i) => (
                <button
                  key={i}
                  className={`text-xs px-3 py-1.5 rounded transition-all ${
                    i === activeTab
                      ? 'bg-amber-500/15 text-amber-400 border border-amber-500/40'
                      : 'bg-white/5 text-gray-500 border border-transparent hover:text-gray-300'
                  }`}
                  onClick={() => setActiveTab(i)}
                >
                  {AGENT_LABELS[o.agent] || o.agent}{' '}
                  <span className="text-gray-600">{(o.durationMs / 1000).toFixed(1)}s</span>
                </button>
              ))}
            </div>
            {outputs[activeTab] && (
              <div
                className="bg-black/40 rounded-lg p-4 text-[13px] leading-relaxed max-h-[400px] overflow-y-auto font-mono whitespace-pre-wrap break-words"
                dangerouslySetInnerHTML={{ __html: renderMd(outputs[activeTab].text || '') }}
              />
            )}
          </div>
        )}

        {/* Flow Log */}
        <div className="mt-6">
          <h3 className="text-xs uppercase tracking-wider text-gray-500 border-b border-white/5 pb-1 mb-3">
            Flow Log ({flows.length})
          </h3>
          {flows.length === 0 ? (
            <div className="text-gray-600 text-xs text-center py-4">No records</div>
          ) : (
            <div className="space-y-0.5">
              {flows.map((f, i) => (
                <div key={i} className="flex gap-2 text-xs py-1 border-b border-white/[0.02] items-baseline">
                  <span className="text-gray-600 min-w-[55px] text-[11px]">{fmtTime(f.at)}</span>
                  <b className="text-gray-300">{f.from}</b>
                  <span className="text-amber-500 font-bold">→</span>
                  <b className="text-gray-300">{f.to}</b>
                  <span className="text-gray-500 flex-1 break-words">{f.remark?.slice(0, 120)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
