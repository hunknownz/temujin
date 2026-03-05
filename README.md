# Temujin - AI Multi-Agent Collaboration for Solo Entrepreneurs

> The steppe conquered the court. In 1206, Temujin united the Mongol tribes and built the largest contiguous empire in history — with no bureaucracy, no court, no red tape. Just scouts, cavalry, and the fastest communication system of the ancient world.

**Temujin** is an AI multi-agent orchestration framework for OpenClaw, designed for **solo entrepreneurs, one-person companies, and hustlers**. While [Edict](https://github.com/cft0808/edict) maps the Tang Dynasty's bureaucratic Three Departments system to agent collaboration, Temujin maps the **Mongol military machine** — built for speed, opportunity capture, and rapid pivots.

## Edict vs Temujin

| | Edict (Three Departments) | **Temujin (Mongol Horde)** |
|---|---|---|
| Metaphor | Imperial court bureaucracy | Nomadic cavalry warfare |
| Philosophy | Quality through review | Speed through action |
| Decision style | Mandatory audit gates | 3-round challenge, then GO |
| Failure handling | Rollback to stable state | **Retreat + migrate to next opportunity** |
| Intelligence | Morning briefing (daily) | **Scout riders (real-time)** |
| Success handling | Task complete | **10x expansion (scale what works)** |
| Best for | Teams, enterprises, compliance | **Solo founders, one-person companies** |
| Deploy | Python + install.sh | **Single Go binary** |

## Architecture: OODA Loop

```
     Khan (You)
         |
    [Command]
         |
    Scout (Tanma) ──→ Advisor (Kurultai) ──→ Vanguard (Charge) ──→ Yam (Report)
         ↑                    |                      |                    |
         └────────────────────┴──── Retreat ─────────┴────────────────────┘
                                   (Migrate)
```

**4 Agents:**

| Agent | Role | Like... |
|-------|------|---------|
| **Tanma** (Scout) | Market intelligence, opportunity detection | A spy behind enemy lines |
| **Advisor** (Counsel) | Challenge assumptions, risk assessment | Devil's advocate with a time limit |
| **Vanguard** (Striker) | Build MVP, execute fast, ship | Cavalry charge — fast and decisive |
| **Yam** (Messenger) | Data tracking, battle reports, alerts | The Mongol postal system |

**8 States (OODA Cycle):**

```
Inbox → Intel → Kurultai → March → Charge → Yam → Loot → Done
                                      ↓
                               Any → Migrate (retreat)
                               Any → Blocked (stuck)
```

Unlike Edict's one-way pipeline, Temujin's states form a **cycle** — you can loop back from Yam to Intel for another round of scouting.

## Quick Start

```bash
# Build
make build

# Start the war room
./temujin serve

# Open http://localhost:7891

# Or use CLI
./temujin raid create "Validate AI writing tool market"
./temujin raid state RAID-xxx Intel "Tanma scouting AI writing market"
./temujin yam RAID-xxx "Found 50+ competitors, market saturated"
./temujin raid retreat RAID-xxx "Too many competitors, pivoting"
```

## CLI Reference

```
temujin serve [port]              Start the war room (default: 7891)
temujin raid create <title>       Launch a new raid
temujin raid state <id> <state>   Update raid state
temujin raid flow <id> <f> <t> <r> Add flow record
temujin raid done <id> [out] [sum] Complete a raid
temujin raid retreat <id> [reason] Retreat (unique to Temujin!)
temujin yam <id> <text> [todos]   Report progress
```

## SOUL Files

Each agent has a personality definition in `souls/`:
- `souls/tanma.md` — Scout intelligence protocols
- `souls/advisor.md` — Challenge framework and verdict format
- `souls/vanguard.md` — Execution rules and retreat protocol
- `souls/yam.md` — Reporting format and alert thresholds

## Roadmap

- [ ] WebSocket real-time updates
- [ ] OpenClaw agent integration
- [ ] Tactical templates (Flash Recon, 72h MVP Raid, Pivot Migration)
- [ ] Intel radar (automated opportunity scanning)
- [ ] 10x expansion mode (scale what works)
- [ ] React frontend upgrade

## License

MIT
