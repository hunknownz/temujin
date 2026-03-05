package dispatch

import (
	"context"
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

const (
	maxRetries    = 3
	maxConcurrent = 2 // Max parallel OpenClaw agent calls
	maxLoops      = 2 // Max OODA loopbacks before forcing Done
)

// BroadcastFunc is called when the dispatcher wants to notify clients.
type BroadcastFunc func(event string, data any)

// Dispatcher polls tasks and auto-dispatches OpenClaw agents through the OODA loop.
type Dispatcher struct {
	store     *store.FileStore
	broadcast BroadcastFunc
	cancel    context.CancelFunc
	ctx       context.Context
	wg        sync.WaitGroup

	mu       sync.Mutex
	inflight map[string]bool
	retries  map[string]int // taskID+state -> retry count
	loops    map[string]int // taskID -> loopback count

	sem chan struct{} // concurrency semaphore
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

func New(s *store.FileStore, bc BroadcastFunc) *Dispatcher {
	if bc == nil {
		bc = func(string, any) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		store:     s,
		broadcast: bc,
		ctx:       ctx,
		cancel:    cancel,
		inflight:  make(map[string]bool),
		retries:   make(map[string]int),
		loops:     make(map[string]int),
		sem:       make(chan struct{}, maxConcurrent),
	}
}

func (d *Dispatcher) Start() {
	log.Printf("[dispatcher] Started (max %d concurrent agents, %d retries, %d loops)", maxConcurrent, maxRetries, maxLoops)
	for {
		select {
		case <-d.ctx.Done():
			log.Println("[dispatcher] Shutting down — waiting for in-flight agents...")
			d.wg.Wait()
			log.Println("[dispatcher] Stopped")
			return
		default:
			d.tick()
			time.Sleep(3 * time.Second)
		}
	}
}

// Stop gracefully shuts down the dispatcher.
// Waits up to 30s for in-flight agents to finish.
func (d *Dispatcher) Stop() {
	d.cancel()
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Println("[dispatcher] All agents finished cleanly")
	case <-time.After(30 * time.Second):
		log.Println("[dispatcher] Shutdown timeout — some agents may still be running")
	}
}

func (d *Dispatcher) tick() {
	tasks, err := d.store.Load()
	if err != nil {
		log.Printf("[dispatcher] Load error: %v", err)
		return
	}

	for _, t := range tasks {
		if t.Archived || t.State == raid.StateDone || t.State == raid.StateMigrate || t.State == raid.StateBlocked {
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
			log.Printf("[dispatcher] %s: max retries (%d) in state %s — blocking", t.ID, maxRetries, t.State)
			d.transitionTo(t.ID, raid.StateBlocked, fmt.Sprintf("Agent failed after %d retries", maxRetries))
			d.addFlowEntry(t.ID, agent, "Blocked", fmt.Sprintf("Max retries in %s", t.State))
			continue
		}
		d.inflight[t.ID] = true
		d.mu.Unlock()

		d.wg.Add(1)
		go d.dispatch(t, agent)
	}
}

func (d *Dispatcher) dispatch(t raid.Task, agent string) {
	defer d.wg.Done()
	defer func() {
		d.mu.Lock()
		delete(d.inflight, t.ID)
		d.mu.Unlock()
	}()

	// Acquire semaphore (limits concurrent agent calls)
	select {
	case d.sem <- struct{}{}:
		defer func() { <-d.sem }()
	case <-d.ctx.Done():
		return
	}

	msg := buildPrompt(t, agent)
	log.Printf("[dispatcher] %s -> agent '%s' (state=%s)", t.ID, agent, t.State)
	d.updateProgress(t.ID, fmt.Sprintf("Dispatching to %s...", agent), t.State)

	result, err := callOpenClaw(d.ctx, agent, msg)
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

	d.appendAgentOutput(t.ID, agent, output, dur)
	d.handleAgentOutput(t, agent, output)
}

func (d *Dispatcher) handleAgentOutput(t raid.Task, agent string, output string) {
	switch t.State {
	case raid.StateIntel:
		d.addFlowEntry(t.ID, "Tanma", "Kurultai", "Intel report delivered")
		d.transitionTo(t.ID, raid.StateKurultai, "Advisor evaluating intel")

	case raid.StateKurultai:
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
		d.addFlowEntry(t.ID, "Vanguard", "Yam", "Execution report delivered")
		if isRetreatSignal(output) {
			d.addFlowEntry(t.ID, "Vanguard", "Migrate", "Execution blocked — retreating")
			d.transitionTo(t.ID, raid.StateMigrate, "Vanguard retreat")
		} else {
			d.transitionTo(t.ID, raid.StateYam, "Yam compiling battle report")
		}

	case raid.StateYam:
		d.addFlowEntry(t.ID, "Yam", "Khan", "Battle report delivered")
		d.transitionTo(t.ID, raid.StateLoot, "Khan reviewing battle report")

		// OODA loopback: if Yam signals SCALE or AT RISK, loop back to Intel
		if d.shouldLoopBack(t.ID, output) {
			d.addFlowEntry(t.ID, "Khan", "Intel", "OODA loopback — re-scouting")
			d.transitionTo(t.ID, raid.StateIntel, "Re-scouting per Yam recommendation")
			log.Printf("[dispatcher] %s: OODA loopback -> Intel", t.ID)
		} else {
			d.addFlowEntry(t.ID, "Khan", "Done", "Loot collected — raid complete")
			d.transitionTo(t.ID, raid.StateDone, "Raid complete")
		}
	}
}

// shouldLoopBack checks if Yam's report signals need for re-scouting.
func (d *Dispatcher) shouldLoopBack(taskID, output string) bool {
	d.mu.Lock()
	loopCount := d.loops[taskID]
	d.mu.Unlock()

	if loopCount >= maxLoops {
		log.Printf("[dispatcher] %s: max loops (%d) reached — completing", taskID, maxLoops)
		return false
	}

	lower := strings.ToLower(output)
	shouldLoop := strings.Contains(lower, "scale") ||
		strings.Contains(lower, "at risk") ||
		strings.Contains(lower, "需要扩展") ||
		strings.Contains(lower, "建议扩展") ||
		strings.Contains(lower, "风险较高")

	if shouldLoop {
		d.mu.Lock()
		d.loops[taskID]++
		d.mu.Unlock()
	}
	return shouldLoop
}

func buildPrompt(t raid.Task, agent string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Raid: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("**ID:** %s\n\n", t.ID))

	// Include accumulated outputs from all previous agents (truncated)
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

	if containsChinese(t.Title) {
		sb.WriteString("\nReply in Chinese (中文回复).\n")
	}

	return sb.String()
}

func callOpenClaw(ctx context.Context, agent string, message string) (*agentResult, error) {
	cmd := exec.CommandContext(ctx, "openclaw", "agent", "--agent", agent, "-m", message, "--json", "--timeout", "300")
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("cancelled: %v", ctx.Err())
		}
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

func isRetreatSignal(output string) bool {
	lower := strings.ToLower(output)
	patterns := []string{
		"decision: no-go", "verdict: no-go",
		"决策: no-go", "裁决: no-go",
		"status: retreat", "verdict: retreat",
		"建议撤退", "建议: 撤退",
		"判定: 不可行", "结论: 不可行",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

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
	d.broadcast("task.updated", map[string]string{"taskId": taskID, "state": newState})
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
