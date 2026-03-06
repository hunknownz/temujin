import type { Task } from '../lib/types'
import { STATE_COLORS } from '../lib/types'
import { isRunning, timeAgo } from '../lib/utils'
import { Pipeline } from './Pipeline'

interface Props {
  task: Task
  onClick: () => void
}

export function RaidCard({ task, onClick }: Props) {
  const color = STATE_COLORS[task.state] || '#6b7280'
  const running = isRunning(task.state)

  return (
    <div
      className="bg-white/[0.04] rounded-lg p-3 cursor-pointer border-l-[3px] hover:border-amber-400 transition-colors group"
      style={{ borderLeftColor: color }}
      onClick={onClick}
    >
      <div className="text-[13px] font-medium leading-snug mb-0.5 group-hover:text-amber-300 transition-colors">
        {running && (
          <span className="inline-block w-2.5 h-2.5 border-2 border-gray-500 border-t-amber-400 rounded-full animate-spin mr-1.5 align-middle" />
        )}
        {task.title}
      </div>
      <Pipeline state={task.state} />
      {task.now && (
        <div className="text-[11px] text-amber-400/80 truncate mt-1">{task.now.slice(0, 60)}</div>
      )}
      <div className="text-[11px] text-gray-500 mt-0.5">
        {task.id} · {timeAgo(task.updatedAt)}
      </div>
    </div>
  )
}
