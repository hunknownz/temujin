import { useState } from 'react'
import { Header } from './components/Header'
import { Kanban } from './components/Kanban'
import { RaidDetail } from './components/RaidDetail'
import { useRaids } from './hooks/useRaids'

export default function App() {
  const { tasks, status, refresh } = useRaids()
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const selectedTask = selectedId ? tasks.find((t) => t.id === selectedId) : null

  return (
    <div className="min-h-screen bg-[#0a0e17] text-gray-200">
      <div className="max-w-[1440px] mx-auto px-4 py-3">
        <Header tasks={tasks} status={status} onLaunched={refresh} />
        <Kanban tasks={tasks} onSelect={setSelectedId} />
        {selectedTask && (
          <RaidDetail task={selectedTask} onClose={() => setSelectedId(null)} onRefresh={refresh} />
        )}
        <footer className="text-center text-gray-600 text-[11px] py-5 mt-6 border-t border-white/5">
          Temujin v0.1.0 — The steppe conquers the court. — OODA: Observe → Orient → Decide → Act
        </footer>
      </div>
    </div>
  )
}
