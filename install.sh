#!/bin/bash
# ══════════════════════════════════════════════════════════════
# Temujin · OpenClaw Multi-Agent System Install Script
# ══════════════════════════════════════════════════════════════
set -e

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OC_HOME="$HOME/.openclaw"
OC_CFG="$OC_HOME/openclaw.json"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

banner() {
  echo ""
  echo -e "${BLUE}============================================${NC}"
  echo -e "${BLUE}  Temujin · Steppe Warfare Agent System${NC}"
  echo -e "${BLUE}  Install Wizard${NC}"
  echo -e "${BLUE}============================================${NC}"
  echo ""
}

log()   { echo -e "${GREEN}[ok] $1${NC}"; }
warn()  { echo -e "${YELLOW}[warn] $1${NC}"; }
error() { echo -e "${RED}[err] $1${NC}"; }
info()  { echo -e "${BLUE}[info] $1${NC}"; }

# ── Step 0: Dependency check ─────────────────────────────────
check_deps() {
  info "Checking dependencies..."

  if ! command -v openclaw &>/dev/null; then
    error "openclaw CLI not found. Install: npm install -g openclaw@latest"
    exit 1
  fi
  log "OpenClaw CLI: $(openclaw --version 2>/dev/null || echo 'OK')"

  if ! command -v claude &>/dev/null; then
    warn "claude CLI not found. Claude CLI backend will not work."
    warn "Install: https://docs.anthropic.com/en/docs/claude-code"
  else
    log "Claude CLI: $(claude --version 2>/dev/null | head -1)"
  fi

  if [ ! -f "$OC_CFG" ]; then
    warn "openclaw.json not found. Creating minimal config..."
    mkdir -p "$OC_HOME"
    echo '{}' > "$OC_CFG"
  fi
  log "Config: $OC_CFG"
}

# ── Step 1: Create Workspaces ────────────────────────────────
create_workspaces() {
  info "Creating agent workspaces..."

  AGENTS=(tanma advisor vanguard yam)
  for agent in "${AGENTS[@]}"; do
    ws="$OC_HOME/workspace-$agent"
    mkdir -p "$ws/skills"

    # Copy SOUL.md
    if [ -f "$REPO_DIR/souls/$agent.md" ]; then
      if [ -f "$ws/SOUL.md" ]; then
        cp "$ws/SOUL.md" "$ws/SOUL.md.bak.$(date +%Y%m%d-%H%M%S)"
      fi
      sed "s|__REPO_DIR__|$REPO_DIR|g" "$REPO_DIR/souls/$agent.md" > "$ws/SOUL.md"
    fi
    log "Workspace: $ws"
  done

  # Common AGENTS.md (work protocol)
  for agent in "${AGENTS[@]}"; do
    cat > "$OC_HOME/workspace-$agent/AGENTS.md" << 'AGENTS_EOF'
# AGENTS.md - Temujin Work Protocol

1. On receiving a task, reply with the task ID and acknowledgment.
2. Output must include: task ID, result, evidence/file paths, blockers.
3. When collaboration is needed, report back to Khan — never call other agents directly.
4. Destructive actions (delete, send, publish) must be flagged and await approval.
5. Time-box: 30 minutes max per sub-task. Retreat if stuck.
AGENTS_EOF
  done
}

# ── Step 2: Register Agents ──────────────────────────────────
register_agents() {
  info "Registering Temujin agents..."

  cp "$OC_CFG" "$OC_CFG.bak.temujin-$(date +%Y%m%d-%H%M%S)"
  log "Config backed up"

  python3 << 'PYEOF'
import json, pathlib

cfg_path = pathlib.Path.home() / '.openclaw' / 'openclaw.json'
cfg = json.loads(cfg_path.read_text())

# Temujin agents with subagent permissions
# Khan (user) -> tanma/advisor/vanguard/yam
# tanma reports back, advisor reports back, vanguard reports back, yam reports back
# Only Khan orchestrates — flat hierarchy, no inter-agent calls
AGENTS = [
    {"id": "tanma",    "subagents": {"allowAgents": []}},
    {"id": "advisor",  "subagents": {"allowAgents": []}},
    {"id": "vanguard", "subagents": {"allowAgents": []}},
    {"id": "yam",      "subagents": {"allowAgents": []}},
]

agents_cfg = cfg.setdefault('agents', {})
agents_list = agents_cfg.get('list', [])
existing_ids = {a['id'] for a in agents_list}

added = 0
for ag in AGENTS:
    ag_id = ag['id']
    ws = str(pathlib.Path.home() / f'.openclaw/workspace-{ag_id}')
    if ag_id not in existing_ids:
        entry = {'id': ag_id, 'workspace': ws, **{k:v for k,v in ag.items() if k != 'id'}}
        agents_list.append(entry)
        added += 1
        print(f'  + added: {ag_id}')
    else:
        print(f'  ~ exists: {ag_id} (skipped)')

agents_cfg['list'] = agents_list

# Configure Claude CLI backend if not already set
defaults = agents_cfg.setdefault('defaults', {})
cli_backends = defaults.setdefault('cliBackends', {})
if 'claude-cli' not in cli_backends:
    import shutil
    claude_path = shutil.which('claude') or '/opt/homebrew/bin/claude'
    cli_backends['claude-cli'] = {'command': claude_path}
    print(f'  + claude-cli backend: {claude_path}')

cfg_path.write_text(json.dumps(cfg, ensure_ascii=False, indent=2))
print(f'Done: {added} agents added')
PYEOF

  log "Agents registered"
}

# ── Step 3: Initialize Data ──────────────────────────────────
init_data() {
  info "Initializing data directory..."

  DATA_DIR="$HOME/.temujin/data"
  mkdir -p "$DATA_DIR"

  if [ ! -f "$DATA_DIR/tasks.json" ]; then
    echo '[]' > "$DATA_DIR/tasks.json"
  fi

  log "Data directory: $DATA_DIR"
}

# ── Step 4: Build binary ─────────────────────────────────────
build_binary() {
  if command -v go &>/dev/null; then
    info "Building temujin binary..."
    cd "$REPO_DIR"
    go build -o temujin ./cmd/temujin
    log "Binary built: $REPO_DIR/temujin"
  else
    warn "Go not found. Skipping binary build."
    warn "Install Go and run: make build"
  fi
}

# ── Step 5: Restart Gateway ──────────────────────────────────
restart_gateway() {
  info "Restarting OpenClaw Gateway..."
  if openclaw gateway restart 2>/dev/null; then
    log "Gateway restarted"
  else
    warn "Gateway restart failed. Run manually: openclaw gateway restart"
  fi
}

# ── Main ─────────────────────────────────────────────────────
banner
check_deps
create_workspaces
register_agents
init_data
build_binary
restart_gateway

echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}  Temujin installed successfully!${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo "Next steps:"
echo "  1. Start the war room:  ./temujin serve"
echo "  2. Open dashboard:      http://localhost:7891"
echo "  3. Create a raid:       ./temujin raid create \"Your mission here\""
echo ""
echo "Test agent connection:"
echo "  openclaw agent --agent tanma -m \"Scout the no-code tools market\""
echo ""
