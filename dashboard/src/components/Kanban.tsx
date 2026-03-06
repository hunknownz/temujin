import type { Task } from '../lib/types'
import { STATES, STATE_COLORS } from '../lib/types'
import { RaidCard } from './RaidCard'

interface Props {
  tasks: Task[]
  onSelect: (id: string) => void
}

export function Kanban({ tasks, onSelect }: Props) {
  const active = tasks.filter((t) => !t.archived)

  return (
    <div className="grid gap-2.5" style={{ gridTemplateColumns: `repeat(${STATES.length}, minmax(160px, 1fr))` }}>
      {STATES.map((stage) => {
        const col = active.filter(
          (t) => t.state === stage || (stage === 'Done' && t.state === 'Migrate')
        )
        return (
          <div key={stage} className="bg-white/[0.03] rounded-lg p-2.5 min-h-[200px]">
            <div className="flex items-center justify-between mb-2">
              <span className="text-[13px] font-semibold" style={{ color: STATE_COLORS[stage] }}>
                {stage}
              </span>
              <span className="text-[11px] text-gray-500 bg-white/5 px-2 py-0.5 rounded-full">
                {col.length}
              </span>
            </div>
            {col.length === 0 ? (
              <div className="text-gray-600 text-xs text-center py-8">Empty</div>
            ) : (
              <div className="flex flex-col gap-2">
                {col.map((t) => (
                  <RaidCard key={t.id} task={t} onClick={() => onSelect(t.id)} />
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
