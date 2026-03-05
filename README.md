# Temujin - AI Multi-Agent Collaboration for Solo Entrepreneurs

> 1300 years ago, the system that defeated the Three Departments court was the steppe cavalry.

**Temujin** is an AI multi-agent orchestration framework built on [OpenClaw](https://openclaw.ai), designed for **solo entrepreneurs, one-person companies, and hustlers**. While [Edict](https://github.com/cft0808/edict) maps the Tang Dynasty's bureaucratic Three Departments system to agent collaboration, Temujin maps the **Mongol military machine** -- built for speed, opportunity capture, and rapid pivots.

## How It Works

You describe a business idea. Temujin automatically dispatches 4 AI agents through an OODA loop:

```
You: "调研AI自动生成短视频带货的可行性，目标：一人公司月入5万"

  [Intel]    Tanma scouts the market        (105s, 2548 chars)
  [Kurultai] Advisor evaluates GO/NO-GO     (66s, 3104 chars)
  [Charge]   Vanguard creates execution plan (67s, 3725 chars)
  [Yam]      Yam compiles battle report     (19s, 744 chars)

  Result: Complete feasibility study with action plan in ~4 minutes
```

Each agent sees all previous agents' outputs, building on each other's work.

## Edict vs Temujin

| | Edict (Three Departments) | **Temujin (Mongol Horde)** |
|---|---|---|
| Metaphor | Imperial court bureaucracy | Nomadic cavalry warfare |
| Philosophy | Quality through review | Speed through action |
| Decision | Mandatory audit gates | 3-round challenge, then GO |
| Failure | Rollback to stable state | **Retreat + migrate** |
| Intelligence | Morning briefing | **Scout riders (real-time)** |
| Best for | Teams, enterprises | **Solo founders** |
| Tech | Python + Redis | **Single Go binary** |
| Deploy | Docker + install.sh | **One binary, zero deps** |

## Quick Start

```bash
# Install OpenClaw (requires Node 22+)
npm install -g openclaw@latest

# Clone and build
git clone https://github.com/hunknownz/temujin.git
cd temujin
make build

# Register agents with OpenClaw
bash install.sh

# Launch the war room (server + auto-dispatcher)
./temujin serve

# Open dashboard
open http://localhost:7891

# Launch a raid (starts OODA loop automatically)
./temujin raid launch "Your business idea here"
```

## Architecture: OODA Loop

```
    Khan (You)
        |
   raid launch "idea"
        |
   [Intel] Tanma (Scout) -----> [Kurultai] Advisor (Evaluate)
        ^                              |
        |                         GO / NO-GO
        |                              |
   [Yam] Yam (Report) <----- [Charge] Vanguard (Execute)
        |                              |
   [Loot] --> [Done]           Any --> [Migrate] (retreat)
```

**4 Agents (via OpenClaw + Claude CLI):**

| Agent | Role | What it does |
|-------|------|-------------|
| **Tanma** | Scout | Market research, competitor analysis, demand signals |
| **Advisor** | Devil's Advocate | Challenges assumptions, scores feasibility, GO/NO-GO |
| **Vanguard** | Striker | Creates execution plan with budget, timeline, tools |
| **Yam** | Messenger | Compiles battle report, key metrics, next action |

**10 States:**

```
Inbox -> Intel -> Kurultai -> March -> Charge -> Yam -> Loot -> Done
                                                   |
                              Any state ---------> Migrate (retreat)
                              Any state ---------> Blocked (stuck, auto-retry 3x)
```

## CLI Reference

```
temujin serve [port]              Start war room + dispatcher (default: 7891)
temujin raid launch <title>       Launch raid with auto OODA loop
temujin raid create <title>       Create raid (manual, stays in Inbox)
temujin raid state <id> <state>   Update raid state
temujin raid retreat <id> [why]   Retreat from a raid
temujin yam <id> <text>           Report progress
temujin version                   Show version
```

## API

```
POST /api/launch-raid     {"title": "..."}           Auto-start OODA loop
POST /api/create-raid     {"title": "..."}           Create in Inbox
POST /api/raid-state      {"taskId","newState"}       Manual state change
POST /api/raid-retreat    {"taskId","reason"}         Retreat
POST /api/raid-action     {"taskId","action","reason"} stop/resume/cancel/archive
GET  /api/live-status                                 All tasks + sync status
GET  /api/pipeline                                    Pipeline stages
GET  /api/healthz                                     Health check
```

## Dashboard

The embedded dashboard at `http://localhost:7891` shows:
- Kanban board with 8 OODA columns
- Real-time raid progress (3s polling)
- Agent output tabs (click any raid card to see full reports)
- Launch raids directly from the UI
- Flow log with timestamp trail

## Project Structure

```
temujin/
  cmd/temujin/main.go           Single binary entry point (CLI + server)
  internal/
    raid/task.go                 Domain types, states, transitions
    dispatch/dispatcher.go       OODA dispatcher (polls + calls OpenClaw agents)
    server/server.go             HTTP API server
    server/dist/index.html       Embedded dashboard (go:embed)
    store/file.go                JSON file store with mutex
  souls/                         Agent personality files (SOUL.md)
    tanma.md                     Scout protocols
    advisor.md                   Evaluation framework
    vanguard.md                  Execution rules
    yam.md                       Reporting format
  install.sh                     OpenClaw agent registration
  examples/                      Demo raid outputs
```

## How the Dispatcher Works

1. Polls `~/.temujin/data/tasks.json` every 3 seconds
2. For each task in an actionable state (Intel/Kurultai/Charge/Yam), calls `openclaw agent --agent <id> -m <prompt> --json`
3. Each agent receives the full accumulated context from all previous agents
4. Agent output is parsed and the task transitions to the next OODA state
5. If an agent fails, retries up to 3 times before blocking
6. Advisor's verdict is checked for NO-GO signals to trigger retreat

## Roadmap

- [x] Single Go binary (CLI + server + dispatcher)
- [x] 4 OpenClaw agents with Claude CLI backend
- [x] Auto OODA loop (Intel -> Kurultai -> Charge -> Yam -> Done)
- [x] Embedded kanban dashboard with agent output viewer
- [x] State validation (enforces ValidTransitions)
- [x] Retry logic with max attempts
- [ ] WebSocket real-time updates (replace 3s polling)
- [ ] React frontend upgrade
- [ ] Tactical templates (Flash Recon, 72h MVP Raid)
- [ ] Intel radar (automated opportunity scanning)
- [ ] OODA cycle loopback (Yam -> Intel for re-scouting)

## License

MIT
