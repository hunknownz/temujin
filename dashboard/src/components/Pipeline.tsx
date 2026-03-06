import { STATES } from '../lib/types'
import { stateIndex } from '../lib/utils'

interface Props {
  state: string
}

export function Pipeline({ state }: Props) {
  const idx = stateIndex(state)
  const isMigrate = state === 'Migrate'

  return (
    <div className="flex gap-0.5 my-1.5">
      {STATES.map((_, i) => {
        let cls = 'bg-white/10'
        if (isMigrate) cls = 'bg-red-500/60'
        else if (i < idx) cls = 'bg-emerald-500'
        else if (i === idx) cls = 'bg-amber-400 animate-pulse'
        return <div key={i} className={`h-1 flex-1 rounded-full ${cls}`} />
      })}
    </div>
  )
}
