export interface FlowEntry {
  at: string
  from: string
  to: string
  remark: string
}

export interface AgentOutput {
  agent: string
  text: string
  at: string
  durationMs: number
}

export interface AgentSummary {
  agent: string
  at: string
  durationMs: number
  chars: number
}

export interface ProgressEntry {
  at: string
  text: string
  state: string
  org: string
}

export interface Task {
  id: string
  title: string
  state: string
  org: string
  now: string
  output: string
  block: string
  flow_log: FlowEntry[]
  agent_outputs: AgentSummary[]
  progress_log: ProgressEntry[]
  archived: boolean
  updatedAt: string
}

export interface TaskDetail extends Omit<Task, 'agent_outputs'> {
  agent_outputs: AgentOutput[]
}

export const STATES = [
  'Inbox', 'Intel', 'Kurultai', 'March', 'Charge', 'Yam', 'Loot', 'Done',
] as const

export const STATE_COLORS: Record<string, string> = {
  Inbox: '#ffd700',
  Intel: '#3b82f6',
  Kurultai: '#8b5cf6',
  March: '#f59e0b',
  Charge: '#ef4444',
  Yam: '#10b981',
  Loot: '#f59e0b',
  Done: '#10b981',
  Migrate: '#8b5cf6',
  Blocked: '#ef4444',
}

export const AGENT_LABELS: Record<string, string> = {
  tanma: 'Tanma (Scout)',
  advisor: 'Advisor (Counsel)',
  vanguard: 'Vanguard (Striker)',
  yam: 'Yam (Messenger)',
}
