package dispatch

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hunknownz/temujin/internal/raid"
	"github.com/hunknownz/temujin/internal/store"
)

const maxRetries = 3

// Dispatcher polls tasks and auto-dispatches OpenClaw agents through the OODA loop.
type Dispatcher struct {
	store    *store.FileStore
	running  bool
	mu       sync.Mutex
	inflight map[string]bool
	retries  map[string]int // taskID+state -> retry count
}

type agentResult struct {
	RunID   string `json:"runId"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Result  struct {
		Payloads []struct {
			Text string `json:"text"`
		} `json:"payloads"`
		Meta struct {
			DurationMs int `json:"durationMs"`
			AgentMeta  struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			} `json:"agentMeta"`
		} `json:"meta"`
	} `json:"result"`
}

func New(s *store.FileStore) *Dispatcher {
	return &Dispatcher{
		store:    s,
		inflight: make(map[string]bool),
		retries:  make(map[string]int),
	}
}

func (d *Dispatcher) Start() {
	d.running = true
	log.Println("[dispatcher] Started — watching for actionable raids")
	for d.running {
		d.tick()
		time.Sleep(3 * time.Second)
	}
	log.Println("[dispatcher] Stopped")
}

func (d *Dispatcher) Stop() {
	d.running = false
}

func (d *Dispatcher) tick() {
	tasks, err := d.store.Load()
	if err != nil {
		log.Printf("[dispatcher] Load error: %v", err)
		return
	}

	for _, t := range tasks {
		if t.Archived || t.State == raid.StateDone || t.State == raid.StateMigrate {
			continue
		}

		d.mu.Lock()
		busy := d.inflight[t.ID]
		d.mu.Unlock()
		if busy {
			continue
		}

		agent, ok := raid.StateAgentMap[t.State]
		if !ok {
			continue
		}

		// Check retry limit
		retryKey := t.ID + ":" + t.State
		d.mu.Lock()
		if d.retries[retryKey] >= maxRetries {
			d.mu.Unlock()
			log.Printf("[dispatcher] %s: max retries (%d) reached for state %s — blocking", t.ID, maxRetries, t.State)
			d.transitionTo(t.ID, raid.StateBlocked, fmt.Sprintf("Agent failed after %d retries", maxRetries))
			d.addFlowEntry(t.ID, agent, "Blocked", fmt.Sprintf("Max retries reached in %s", t.State))
			continue
		}
		d.inflight[t.ID] = true
		d.mu.Unlock()

		go d.dispatch(t, agent)
	}
}

func (d *Dispatcher) dispatch(t raid.Task, agent string) {
	defer func() {
		d.mu.Lock()
		delete(d.inflight, t.ID)
		d.mu.Unlock()
	}()

	msg := buildPrompt(t, agent)
	log.Printf("[dispatcher] %s -> agent '%s' (state=%s)", t.ID, agent, t.State)

	d.updateProgress(t.ID, fmt.Sprintf("Dispatching to %s...", agent), t.State)

	result, err := callOpenClaw(agent, msg)
	if err != nil {
		retryKey := t.ID + ":" + t.State
		d.mu.Lock()
		d.retries[retryKey]++
		attempt := d.retries[retryKey]
		d.mu.Unlock()
		log.Printf("[dispatcher] ERROR %s -> %s (attempt %d/%d): %v", t.ID, agent, attempt, maxRetries, err)
		d.updateProgress(t.ID, fmt.Sprintf("Agent %s failed (attempt %d/%d)", agent, attempt, maxRetries), t.State)
		return
	}

	output := ""
	if len(result.Result.Payloads) > 0 {
		output = result.Result.Payloads[0].Text
	}
	if output == "" {
		log.Printf("[dispatcher] WARN %s: agent '%s' returned empty", t.ID, agent)
		return
	}

	dur := result.Result.Meta.DurationMs
	log.Printf("[dispatcher] %s <- agent '%s' (%dms, %d chars)", t.ID, agent, dur, len(output))

	// Clear retry counter on success
	retryKey := t.ID + ":" + t.State
	d.mu.Lock()
	delete(d.retries, retryKey)
	d.mu.Unlock()

	// Accumulate this agent's output into the task's AgentOutputs
	d.appendAgentOutput(t.ID, agent, output, dur)

	d.handleAgentOutput(t, agent, output)
}

func (d *Dispatcher) handleAgentOutput(t raid.Task, agent string, output string) {
	switch t.State {
	case raid.StateIntel:
		// Tanma → Kurultai
		d.addFlowEntry(t.ID, "Tanma", "Kurultai", "Intel report delivered")
		d.transitionTo(t.ID, raid.StateKurultai, "Advisor evaluating intel")

	case raid.StateKurultai:
		// Advisor → GO/NO-GO
		d.addFlowEntry(t.ID, "Advisor", "Khan", verdictSummary(output))
		if isRetreatSignal(output) {
			d.addFlowEntry(t.ID, "Khan", "Migrate", "Retreat per Advisor NO-GO")
			d.transitionTo(t.ID, raid.StateMigrate, "Advisor verdict: NO-GO")
		} else {
			d.addFlowEntry(t.ID, "Khan", "Vanguard", "Khan approves — execute")
			d.transitionTo(t.ID, raid.StateMarch, "Preparing strike")
			d.transitionTo(t.ID, raid.StateCharge, "Vanguard executing")
		}

	case raid.StateCharge:
		// Vanguard → Yam (or retreat)
		d.addFlowEntry(t.ID, "Vanguard", "Yam", "Execution report delivered")
		if isRetreatSignal(output) {
			d.addFlowEntry(t.ID, "Vanguard", "Migrate", "Execution blocked — retreating")
			d.transitionTo(t.ID, raid.StateMigrate, "Vanguard retreat")
		} else {
			d.transitionTo(t.ID, raid.StateYam, "Yam compiling battle report")
		}

	case raid.StateYam:
		// Yam → Loot → Done
		d.addFlowEntry(t.ID, "Yam", "Khan", "Battle report delivered")
		d.transitionTo(t.ID, raid.StateLoot, "Khan reviewing battle report")
		d.addFlowEntry(t.ID, "Khan", "Done", "Loot collected — raid complete")
		d.transitionTo(t.ID, raid.StateDone, "Raid complete")
	}
}

// buildPrompt assembles the full context for an agent call.
// Key design: each agent sees ALL previous agents' outputs, not just the last one.
func buildPrompt(t raid.Task, agent string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Raid: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("**ID:** %s\n\n", t.ID))

	// Include accumulated outputs from all previous agents (truncated to avoid token overflow)
	if len(t.AgentOutputs) > 0 {
		sb.WriteString("---\n\n")
		for _, ao := range t.AgentOutputs {
			label := strings.ToUpper(ao.Agent)
			text := ao.Text
			if len(text) > 3000 {
				text = text[:3000] + "\n\n[... truncated for brevity ...]"
			}
			sb.WriteString(fmt.Sprintf("## %s Report\n\n%s\n\n---\n\n", label, text))
		}
	}

	// Agent-specific mission
	sb.WriteString("## Your Mission\n\n")
	switch agent {
	case raid.AgentTanma:
		sb.WriteString("You are Tanma (scout). Investigate this business opportunity. " +
			"Search for market data, competitors, demand signals, risks. " +
			"Reply with your structured Tanma Intel Report format.\n")

	case raid.AgentAdvisor:
		sb.WriteString("You are the Advisor. Read ALL the reports above carefully. " +
			"Challenge assumptions, score each dimension (Opportunity/Competition/Cost/Risk, 1-5 each). " +
			"If total < 10, verdict is NO-GO. " +
			"Reply with your structured Advisor Evaluation format.\n")

	case raid.AgentVanguard:
		sb.WriteString("You are the Vanguard. The Advisor approved this raid. " +
			"Based on ALL reports above, create a concrete, time-boxed execution plan. " +
			"Include tools, budget, success metrics, and retreat triggers. " +
			"Set Status: CHARGE if executable, or Status: RETREAT if blocked.\n")

	case raid.AgentYam:
		sb.WriteString("You are the Yam (messenger). Read ALL reports above. " +
			"Compile a concise battle report summarizing the full raid cycle. " +
			"Include phase summary, key numbers, and one concrete next action. " +
			"Keep it under 300 words.\n")
	}

	// Language hint
	if containsChinese(t.Title) {
		sb.WriteString("\nReply in Chinese (中文回复).\n")
	}

	return sb.String()
}

func callOpenClaw(agent string, message string) (*agentResult, error) {
	cmd := exec.Command("openclaw", "agent", "--agent", agent, "-m", message, "--json", "--timeout", "300")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, err
	}

	var result agentResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("JSON parse: %v (raw: %s)", err, truncate(string(out), 200))
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("agent status=%s summary=%s", result.Status, result.Summary)
	}

	return &result, nil
}

// isRetreatSignal detects explicit NO-GO/RETREAT verdicts.
func isRetreatSignal(output string) bool {
	lower := strings.ToLower(output)
	patterns := []string{
		"decision: no-go",
		"verdict: no-go",
		"决策: no-go",
		"裁决: no-go",
		"status: retreat",
		"verdict: retreat",
		"建议撤退",
		"建议: 撤退",
		"判定: 不可行",
		"结论: 不可行",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// verdictSummary extracts a short verdict line from advisor output.
func verdictSummary(output string) string {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "decision:") || strings.Contains(lower, "verdict:") ||
			strings.Contains(lower, "决策:") || strings.Contains(lower, "裁决:") {
			return strings.TrimSpace(line)
		}
	}
	return "Evaluation complete"
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// --- Store helpers ---

func (d *Dispatcher) transitionTo(taskID, newState, comment string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				// Validate transition
				if !isValidTransition(tasks[i].State, newState) {
					log.Printf("[dispatcher] INVALID transition %s: %s -> %s", taskID, tasks[i].State, newState)
					return tasks
				}
				tasks[i].State = newState
				if org, ok := raid.StateOrgMap[newState]; ok {
					tasks[i].Org = org
				}
				tasks[i].Now = comment
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	log.Printf("[dispatcher] %s -> state=%s", taskID, newState)
}

func isValidTransition(from, to string) bool {
	valid, ok := raid.ValidTransitions[from]
	if !ok {
		return false
	}
	for _, v := range valid {
		if v == to {
			return true
		}
	}
	return false
}

func (d *Dispatcher) addFlowEntry(taskID, from, to, remark string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
					At: raid.NowISO(), From: from, To: to, Remark: remark,
				})
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
}

func (d *Dispatcher) appendAgentOutput(taskID, agent, text string, durationMs int) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].AgentOutputs = append(tasks[i].AgentOutputs, raid.AgentOutput{
					Agent:      agent,
					Text:       text,
					At:         raid.NowISO(),
					DurationMs: durationMs,
				})
				// Also keep Output field updated with latest for backward compat
				tasks[i].Output = text
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
}

func (d *Dispatcher) updateProgress(taskID, text, state string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].Now = text
				tasks[i].ProgressLog = append(tasks[i].ProgressLog, raid.ProgressEntry{
					At:    raid.NowISO(),
					Text:  text,
					State: state,
					Org:   tasks[i].Org,
				})
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
