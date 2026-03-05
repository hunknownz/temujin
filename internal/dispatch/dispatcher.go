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

// Dispatcher polls tasks and auto-dispatches OpenClaw agents.
type Dispatcher struct {
	store    *store.FileStore
	running  bool
	mu       sync.Mutex
	// Track which tasks are currently being dispatched (prevent double dispatch)
	inflight map[string]bool
}

// OpenClaw agent response
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
	}
}

// Start begins the dispatch loop. Call in a goroutine.
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
			// States like Inbox, Loot — need Khan (user) action, not agent dispatch
			continue
		}

		// Auto-dispatch: this state has an agent assigned
		d.mu.Lock()
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

	// Update progress: dispatching
	d.updateProgress(t.ID, fmt.Sprintf("Dispatching to %s...", agent), t.State)

	result, err := callOpenClaw(agent, msg)
	if err != nil {
		log.Printf("[dispatcher] ERROR %s -> %s: %v", t.ID, agent, err)
		d.addFlowEntry(t.ID, agent, "Khan", fmt.Sprintf("Agent error: %v", err))
		d.updateProgress(t.ID, fmt.Sprintf("Agent %s error: %v", agent, err), t.State)
		return
	}

	output := ""
	if len(result.Result.Payloads) > 0 {
		output = result.Result.Payloads[0].Text
	}
	if output == "" {
		log.Printf("[dispatcher] WARN %s: agent '%s' returned empty output", t.ID, agent)
		return
	}

	log.Printf("[dispatcher] %s <- agent '%s' (%dms, %d chars)",
		t.ID, agent, result.Result.Meta.DurationMs, len(output))

	// Store agent output and transition to next state
	d.handleAgentOutput(t, agent, output)
}

func (d *Dispatcher) handleAgentOutput(t raid.Task, agent string, output string) {
	switch t.State {
	case raid.StateIntel:
		// Tanma finished scouting -> move to Kurultai for evaluation
		d.addFlowEntry(t.ID, "Tanma", "Kurultai", truncate(output, 200))
		d.storeOutput(t.ID, output)
		d.transitionState(t.ID, raid.StateKurultai, "Advisor evaluating intel report")

	case raid.StateKurultai:
		// Advisor finished evaluation -> check GO/NO-GO
		d.addFlowEntry(t.ID, "Advisor", "Khan", truncate(output, 200))
		d.storeOutput(t.ID, output)
		if isRetreatSignal(output) {
			d.transitionState(t.ID, raid.StateMigrate, "Advisor verdict: NO-GO — retreating")
			d.addFlowEntry(t.ID, "Khan", "Migrate", "Retreat per Advisor's recommendation")
		} else {
			d.transitionState(t.ID, raid.StateMarch, "Preparing strike based on Advisor GO verdict")
			// March -> Charge immediately (no separate agent for March)
			d.addFlowEntry(t.ID, "Khan", "Vanguard", "Execute the raid plan")
			d.transitionState(t.ID, raid.StateCharge, "Vanguard executing raid")
		}

	case raid.StateCharge:
		// Vanguard finished execution -> move to Yam for reporting
		d.addFlowEntry(t.ID, "Vanguard", "Yam", truncate(output, 200))
		d.storeOutput(t.ID, output)
		if isRetreatSignal(output) {
			d.transitionState(t.ID, raid.StateMigrate, "Vanguard retreat: "+truncate(output, 80))
			d.addFlowEntry(t.ID, "Vanguard", "Migrate", "Execution failed, retreating")
		} else {
			d.transitionState(t.ID, raid.StateYam, "Yam collecting battle data")
		}

	case raid.StateYam:
		// Yam finished reporting -> move to Loot
		d.addFlowEntry(t.ID, "Yam", "Khan", truncate(output, 200))
		d.storeOutput(t.ID, output)
		d.transitionState(t.ID, raid.StateLoot, "Battle report ready — Khan reviewing results")
		// Auto-complete to Done (for MVP, Khan auto-approves loot)
		d.addFlowEntry(t.ID, "Khan", "Done", "Raid complete — loot collected")
		d.completeDone(t.ID, output)
	}
}

// buildPrompt creates the message to send to an agent based on the current raid context.
func buildPrompt(t raid.Task, agent string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("RAID ID: %s\n", t.ID))
	sb.WriteString(fmt.Sprintf("RAID TITLE: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("CURRENT STATE: %s\n", t.State))

	// Include previous output as context
	if t.Output != "" {
		sb.WriteString(fmt.Sprintf("\n--- PREVIOUS INTEL ---\n%s\n--- END INTEL ---\n", t.Output))
	}

	// Include recent flow log for context
	if len(t.FlowLog) > 0 {
		sb.WriteString("\n--- FLOW LOG (recent) ---\n")
		start := len(t.FlowLog) - 5
		if start < 0 {
			start = 0
		}
		for _, f := range t.FlowLog[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s -> %s: %s\n", f.At, f.From, f.To, f.Remark))
		}
		sb.WriteString("--- END FLOW LOG ---\n")
	}

	switch agent {
	case raid.AgentTanma:
		sb.WriteString(fmt.Sprintf("\nMISSION: Scout and investigate the opportunity described in '%s'. "+
			"Search for market size, competitors, demand signals, risks. "+
			"Reply with a structured intel report. Keep it under 500 words. "+
			"Use Chinese if the raid title is in Chinese.\n", t.Title))

	case raid.AgentAdvisor:
		sb.WriteString(fmt.Sprintf("\nMISSION: Evaluate the intel report above for raid '%s'. "+
			"Challenge assumptions, assess feasibility. "+
			"Give a clear verdict: GO / NO-GO / CONDITIONAL. "+
			"Use the Evaluation Framework (Opportunity/Competition/Cost/Risk). "+
			"End with your structured Verdict Format. "+
			"Use Chinese if the intel report is in Chinese.\n", t.Title))

	case raid.AgentVanguard:
		sb.WriteString(fmt.Sprintf("\nMISSION: Execute the approved raid plan for '%s'. "+
			"Based on the intel and advisor verdict above, create a concrete action plan and execute it. "+
			"For this MVP demo, describe the specific steps you would take, tools you would use, "+
			"and estimated timeline. Provide a concrete execution report. "+
			"If you see blockers, say RETREAT. "+
			"Use Chinese if previous reports are in Chinese.\n", t.Title))

	case raid.AgentYam:
		sb.WriteString(fmt.Sprintf("\nMISSION: Generate a battle report for raid '%s'. "+
			"Summarize the full raid cycle: what was scouted, what was evaluated, what was executed. "+
			"Use the Battle Report format. Assess: On Track / At Risk / Failed. "+
			"Recommend next action. "+
			"Use Chinese if previous reports are in Chinese.\n", t.Title))
	}

	return sb.String()
}

// callOpenClaw invokes `openclaw agent --agent <id> -m <msg> --json`
func callOpenClaw(agent string, message string) (*agentResult, error) {
	cmd := exec.Command("openclaw", "agent", "--agent", agent, "-m", message, "--json", "--timeout", "120")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, err
	}

	var result agentResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("JSON parse error: %v (raw: %s)", err, truncate(string(out), 200))
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("agent returned status=%s summary=%s", result.Status, result.Summary)
	}

	return &result, nil
}

// isRetreatSignal checks if the agent output explicitly signals a retreat/NO-GO verdict.
// Must be strict to avoid false positives from agents merely mentioning retreat as a concept.
func isRetreatSignal(output string) bool {
	lower := strings.ToLower(output)
	// Look for explicit verdict patterns, not mere mentions
	verdictPatterns := []string{
		"decision: no-go",
		"verdict: no-go",
		"决策: no-go",
		"裁决: no-go",
		"verdict: retreat",
		"status: retreat",
		"建议撤退",
		"建议: 撤退",
		"判定: 不可行",
		"结论: 不可行",
	}
	for _, p := range verdictPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// --- Store helpers ---

func (d *Dispatcher) transitionState(taskID, newState, comment string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
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

func (d *Dispatcher) storeOutput(taskID, output string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].Output = output
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

func (d *Dispatcher) completeDone(taskID, output string) {
	d.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].State = raid.StateDone
				tasks[i].Output = output
				tasks[i].Now = "Raid complete"
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	log.Printf("[dispatcher] %s -> DONE", taskID)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
