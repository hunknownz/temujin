package raid

import (
	"fmt"
	"time"
)

// OODA States
const (
	StateInbox    = "Inbox"    // User submitted
	StateIntel    = "Intel"    // Scout investigating
	StateKurultai = "Kurultai" // Council evaluating
	StateMarch    = "March"    // Preparing to strike
	StateCharge   = "Charge"   // Executing
	StateYam      = "Yam"      // Reporting results
	StateLoot     = "Loot"     // Collecting rewards
	StateDone     = "Done"     // Completed
	StateMigrate  = "Migrate"  // Retreated / pivoted
	StateBlocked  = "Blocked"  // Stuck
)

// Agent IDs
const (
	AgentTanma    = "tanma"    // Scout
	AgentAdvisor  = "advisor"  // Advisor / Devil's advocate
	AgentVanguard = "vanguard" // Striker
	AgentYam      = "yam"      // Signal / Messenger
)

// StateOrgMap maps state to responsible org
var StateOrgMap = map[string]string{
	StateInbox:    "Khan",
	StateIntel:    "Tanma",
	StateKurultai: "Kurultai",
	StateMarch:    "Vanguard",
	StateCharge:   "Vanguard",
	StateYam:      "Yam",
	StateLoot:     "Khan",
	StateDone:     "Done",
	StateMigrate:  "Migrate",
	StateBlocked:  "Blocked",
}

// StateAgentMap maps state to the agent that handles it
var StateAgentMap = map[string]string{
	StateIntel:    AgentTanma,
	StateKurultai: AgentAdvisor,
	StateMarch:    AgentVanguard,
	StateCharge:   AgentVanguard,
	StateYam:      AgentYam,
}

// Pipeline stages for UI
var Pipeline = []Stage{
	{Key: StateInbox, Dept: "Khan", Icon: "crown", Action: "Command"},
	{Key: StateIntel, Dept: "Tanma", Icon: "eye", Action: "Scout"},
	{Key: StateKurultai, Dept: "Kurultai", Icon: "shield", Action: "Evaluate"},
	{Key: StateMarch, Dept: "Vanguard", Icon: "horse", Action: "Prepare"},
	{Key: StateCharge, Dept: "Vanguard", Icon: "swords", Action: "Strike"},
	{Key: StateYam, Dept: "Yam", Icon: "scroll", Action: "Report"},
	{Key: StateLoot, Dept: "Khan", Icon: "gem", Action: "Collect"},
	{Key: StateDone, Dept: "Done", Icon: "check", Action: "Complete"},
}

// Valid state transitions (OODA cycle allows more flexibility than Edict)
var ValidTransitions = map[string][]string{
	StateInbox:    {StateIntel, StateCharge},             // Can skip scout for urgent raids
	StateIntel:    {StateKurultai, StateCharge, StateMigrate}, // Scout can trigger retreat
	StateKurultai: {StateMarch, StateCharge, StateMigrate},    // Council can approve or retreat
	StateMarch:    {StateCharge, StateMigrate},
	StateCharge:   {StateYam, StateMigrate, StateBlocked},
	StateYam:      {StateLoot, StateIntel, StateMigrate}, // Can loop back to scout
	StateLoot:     {StateDone, StateIntel},                // Success: done or expand (re-scout)
	StateBlocked:  {StateCharge, StateMigrate},            // Resume or retreat
}

type Stage struct {
	Key    string `json:"key"`
	Dept   string `json:"dept"`
	Icon   string `json:"icon"`
	Action string `json:"action"`
}

type FlowEntry struct {
	At     string `json:"at"`
	From   string `json:"from"`
	To     string `json:"to"`
	Remark string `json:"remark"`
}

type TodoItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // not-started, in-progress, completed
	Detail string `json:"detail,omitempty"`
}

type ProgressEntry struct {
	At         string     `json:"at"`
	Agent      string     `json:"agent"`
	AgentLabel string     `json:"agentLabel"`
	Text       string     `json:"text"`
	Todos      []TodoItem `json:"todos,omitempty"`
	State      string     `json:"state"`
	Org        string     `json:"org"`
	Tokens     int        `json:"tokens,omitempty"`
	Cost       float64    `json:"cost,omitempty"`
	Elapsed    int        `json:"elapsed,omitempty"`
}

type Task struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	State       string          `json:"state"`
	Org         string          `json:"org"`
	Now         string          `json:"now"`
	Block       string          `json:"block"`
	Output      string          `json:"output"`
	FlowLog     []FlowEntry     `json:"flow_log"`
	Todos       []TodoItem      `json:"todos,omitempty"`
	ProgressLog []ProgressEntry `json:"progress_log,omitempty"`
	Archived    bool            `json:"archived"`
	ArchivedAt  string          `json:"archivedAt,omitempty"`
	UpdatedAt   string          `json:"updatedAt"`
	PrevState   string          `json:"_prev_state,omitempty"`
}

func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func NewTaskID() string {
	now := time.Now()
	return fmt.Sprintf("RAID-%s-%s%03d", now.Format("20060102"), now.Format("150405"), now.Nanosecond()/1e6)
}
