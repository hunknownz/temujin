package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hunknownz/temujin/internal/raid"
	"github.com/hunknownz/temujin/internal/store"
)

//go:embed dist
var distFS embed.FS

type Server struct {
	store     *store.FileStore
	wsClients map[*wsClient]bool
	wsMu      sync.Mutex
	mux       *http.ServeMux
}

type wsClient struct {
	send chan []byte
}

func New(s *store.FileStore) *Server {
	srv := &Server{
		store:     s,
		wsClients: make(map[*wsClient]bool),
	}
	srv.mux = http.NewServeMux()
	srv.routes()
	return srv
}

// BroadcastEvent exposes broadcast to external callers (e.g. dispatcher).
func (s *Server) BroadcastEvent(event string, data any) {
	s.broadcast(event, data)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("GET /api/live-status", s.handleLiveStatus)
	s.mux.HandleFunc("GET /api/pipeline", s.handlePipeline)
	s.mux.HandleFunc("POST /api/create-raid", s.handleCreateRaid)
	s.mux.HandleFunc("POST /api/launch-raid", s.handleLaunchRaid)
	s.mux.HandleFunc("POST /api/raid-state", s.handleRaidState)
	s.mux.HandleFunc("POST /api/raid-flow", s.handleRaidFlow)
	s.mux.HandleFunc("POST /api/raid-done", s.handleRaidDone)
	s.mux.HandleFunc("POST /api/raid-retreat", s.handleRaidRetreat)
	s.mux.HandleFunc("POST /api/raid-progress", s.handleRaidProgress)
	s.mux.HandleFunc("POST /api/raid-action", s.handleRaidAction)
	s.mux.HandleFunc("GET /api/raid-detail", s.handleRaidDetail)
	s.mux.HandleFunc("GET /api/healthz", s.handleHealth)

	// WebSocket
	s.mux.HandleFunc("GET /api/ws", s.handleWS)

	// SPA fallback
	s.mux.HandleFunc("/", s.handleSPA)
}

// --- API Handlers ---

func (s *Server) handleLiveStatus(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.Load()
	if err != nil {
		sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Return lightweight task summaries (omit heavy agent output text)
	type agentSummary struct {
		Agent      string `json:"agent"`
		At         string `json:"at"`
		DurationMs int    `json:"durationMs"`
		Chars      int    `json:"chars"`
	}
	type taskSummary struct {
		raid.Task
		AgentOutputs []agentSummary `json:"agent_outputs,omitempty"`
	}
	summaries := make([]taskSummary, len(tasks))
	for i, t := range tasks {
		ts := taskSummary{Task: t}
		ts.Task.AgentOutputs = nil // clear from embedded
		ts.Task.Output = ""       // omit last raw output
		for _, ao := range t.AgentOutputs {
			ts.AgentOutputs = append(ts.AgentOutputs, agentSummary{
				Agent: ao.Agent, At: ao.At, DurationMs: ao.DurationMs, Chars: len(ao.Text),
			})
		}
		summaries[i] = ts
	}
	sendJSON(w, 200, map[string]any{
		"tasks":      summaries,
		"syncStatus": map[string]any{"ok": true},
	})
}

func (s *Server) handlePipeline(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, 200, raid.Pipeline)
}

func (s *Server) handleCreateRaid(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		ID    string `json:"id,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	title := sanitizeText(req.Title, 80)
	if len(title) < 6 {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "title too short (min 6 chars)"})
		return
	}

	taskID := req.ID
	if taskID == "" {
		taskID = raid.NewTaskID()
	}
	now := raid.NowISO()

	task := raid.Task{
		ID:    taskID,
		Title: title,
		State: raid.StateInbox,
		Org:   raid.StateOrgMap[raid.StateInbox],
		Now:   "Awaiting Khan's command",
		FlowLog: []raid.FlowEntry{{
			At: now, From: "Khan", To: "Inbox", Remark: "New raid: " + title,
		}},
		UpdatedAt: now,
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		return append([]raid.Task{task}, tasks...)
	})
	s.broadcast("task.created", task)

	sendJSON(w, 200, map[string]any{"ok": true, "taskId": taskID})
}

func (s *Server) handleLaunchRaid(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	title := sanitizeText(req.Title, 200)
	if len(title) < 6 {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "title too short (min 6 chars)"})
		return
	}

	taskID := raid.NewTaskID()
	now := raid.NowISO()

	task := raid.Task{
		ID:    taskID,
		Title: title,
		State: raid.StateIntel,
		Org:   raid.StateOrgMap[raid.StateIntel],
		Now:   "Tanma dispatched — scouting in progress",
		FlowLog: []raid.FlowEntry{
			{At: now, From: "Khan", To: "Inbox", Remark: "New raid: " + title},
			{At: now, From: "Khan", To: "Intel", Remark: "Khan orders: send Tanma to scout"},
		},
		UpdatedAt: now,
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		return append([]raid.Task{task}, tasks...)
	})
	s.broadcast("raid.launched", task)

	sendJSON(w, 200, map[string]any{"ok": true, "taskId": taskID})
}

func (s *Server) handleRaidState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID   string `json:"taskId"`
		NewState string `json:"newState"`
		Comment  string `json:"comment"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	err := s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == req.TaskID {
				tasks[i].State = req.NewState
				if org, ok := raid.StateOrgMap[req.NewState]; ok {
					tasks[i].Org = org
				}
				if req.Comment != "" {
					tasks[i].Now = sanitizeText(req.Comment, 120)
				}
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	if err != nil {
		sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.broadcast("task.updated", map[string]string{"taskId": req.TaskID, "state": req.NewState})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"taskId"`
		From   string `json:"from"`
		To     string `json:"to"`
		Remark string `json:"remark"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == req.TaskID {
				tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
					At: raid.NowISO(), From: req.From, To: req.To,
					Remark: sanitizeText(req.Remark, 120),
				})
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidDone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID  string `json:"taskId"`
		Output  string `json:"output"`
		Summary string `json:"summary"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == req.TaskID {
				tasks[i].State = raid.StateDone
				tasks[i].Output = req.Output
				tasks[i].Now = sanitizeText(req.Summary, 120)
				tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
					At: raid.NowISO(), From: tasks[i].Org, To: "Khan",
					Remark: "Raid complete: " + sanitizeText(req.Summary, 80),
				})
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidRetreat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"taskId"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == req.TaskID {
				tasks[i].State = raid.StateMigrate
				tasks[i].Now = "Retreated: " + sanitizeText(req.Reason, 100)
				tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
					At: raid.NowISO(), From: tasks[i].Org, To: "Migrate",
					Remark: "Retreat: " + sanitizeText(req.Reason, 80),
				})
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"taskId"`
		Text   string `json:"text"`
		Todos  string `json:"todos"` // pipe-delimited
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == req.TaskID {
				tasks[i].Now = sanitizeText(req.Text, 120)
				if req.Todos != "" {
					tasks[i].Todos = parseTodos(req.Todos)
				}
				entry := raid.ProgressEntry{
					At:    raid.NowISO(),
					Text:  sanitizeText(req.Text, 120),
					Todos: tasks[i].Todos,
					State: tasks[i].State,
					Org:   tasks[i].Org,
				}
				tasks[i].ProgressLog = append(tasks[i].ProgressLog, entry)
				if len(tasks[i].ProgressLog) > 100 {
					tasks[i].ProgressLog = tasks[i].ProgressLog[len(tasks[i].ProgressLog)-100:]
				}
				tasks[i].UpdatedAt = raid.NowISO()
				break
			}
		}
		return tasks
	})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"taskId"`
		Action string `json:"action"` // stop, cancel, resume, archive
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	s.store.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID != req.TaskID {
				continue
			}
			switch req.Action {
			case "stop":
				tasks[i].PrevState = tasks[i].State
				tasks[i].State = raid.StateBlocked
				tasks[i].Block = req.Reason
			case "resume":
				prev := tasks[i].PrevState
				if prev == "" {
					prev = raid.StateCharge
				}
				tasks[i].State = prev
				tasks[i].PrevState = ""
				tasks[i].Block = ""
			case "cancel":
				tasks[i].State = raid.StateMigrate
				tasks[i].Now = "Cancelled: " + req.Reason
			case "archive":
				tasks[i].Archived = true
				tasks[i].ArchivedAt = raid.NowISO()
			case "unarchive":
				tasks[i].Archived = false
				tasks[i].ArchivedAt = ""
			}
			tasks[i].UpdatedAt = raid.NowISO()
			break
		}
		return tasks
	})
	sendJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleRaidDetail(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "missing ?id= param"})
		return
	}
	tasks, err := s.store.Load()
	if err != nil {
		sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	for _, t := range tasks {
		if t.ID == taskID {
			sendJSON(w, 200, map[string]any{"ok": true, "task": t})
			return
		}
	}
	sendJSON(w, 404, map[string]any{"ok": false, "error": "task not found"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, 200, map[string]any{"ok": true, "version": "0.1.0"})
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	// Try to serve from embedded dist/
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		http.Error(w, "frontend not built", 500)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	// Try exact file first
	if f, err := sub.Open(path); err == nil {
		f.Close()
		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
		return
	}
	// SPA fallback
	r.URL.Path = "/"
	http.FileServer(http.FS(sub)).ServeHTTP(w, r)
}

// --- WebSocket ---

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}
	client := &wsClient{send: make(chan []byte, 64)}
	s.wsMu.Lock()
	s.wsClients[client] = true
	s.wsMu.Unlock()
	log.Printf("[ws] client connected (%d total)", len(s.wsClients))

	// Writer goroutine
	go func() {
		defer conn.Close()
		for msg := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Reader goroutine (just drain pings/close)
	go func() {
		defer func() {
			s.wsMu.Lock()
			delete(s.wsClients, client)
			s.wsMu.Unlock()
			close(client.send)
			log.Printf("[ws] client disconnected (%d total)", len(s.wsClients))
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

func (s *Server) broadcast(event string, data any) {
	msg, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for client := range s.wsClients {
		select {
		case client.send <- msg:
		default:
			// Client too slow, drop message
		}
	}
}

// --- Helpers ---

func sendJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func sanitizeText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

func parseTodos(pipe string) []raid.TodoItem {
	var todos []raid.TodoItem
	for i, item := range strings.Split(pipe, "|") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		status := "not-started"
		title := item
		if strings.HasSuffix(item, "\u2705") { // checkmark
			status = "completed"
			title = strings.TrimSuffix(item, "\u2705")
		} else if strings.HasSuffix(item, "\U0001f504") { // spinner
			status = "in-progress"
			title = strings.TrimSuffix(item, "\U0001f504")
		}
		todos = append(todos, raid.TodoItem{
			ID:     fmt.Sprintf("%d", i+1),
			Title:  strings.TrimSpace(title),
			Status: status,
		})
	}
	return todos
}
