package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hunknownz/temujin/internal/dispatch"
	"github.com/hunknownz/temujin/internal/raid"
	"github.com/hunknownz/temujin/internal/server"
	"github.com/hunknownz/temujin/internal/store"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	dataDir := getDataDir()
	s := store.NewFileStore(dataDir)

	switch os.Args[1] {
	case "serve":
		port := "7891"
		if len(os.Args) > 2 {
			port = os.Args[2]
		}
		srv := server.New(s)

		// Start the dispatcher in background (with WS broadcast)
		disp := dispatch.New(s, srv.BroadcastEvent)
		go disp.Start()
		addr := "127.0.0.1:" + port
		httpSrv := &http.Server{Addr: addr, Handler: srv}

		// Graceful shutdown on SIGINT/SIGTERM
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			log.Printf("Received %s — shutting down...", sig)
			disp.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpSrv.Shutdown(ctx)
		}()

		log.Printf("Temujin v%s starting on http://%s (dispatcher active)", version, addr)
		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
		log.Println("Temujin stopped.")

	case "raid":
		if len(os.Args) < 3 {
			fmt.Println("Usage: temujin raid <create|state|flow|done|retreat|progress> ...")
			os.Exit(1)
		}
		handleRaidCLI(s, os.Args[2], os.Args[3:])

	case "yam":
		// Shortcut for progress report
		if len(os.Args) < 4 {
			fmt.Println("Usage: temujin yam <task-id> <text> [todos-pipe]")
			os.Exit(1)
		}
		todos := ""
		if len(os.Args) > 4 {
			todos = os.Args[4]
		}
		handleProgress(s, os.Args[2], os.Args[3], todos)

	case "version":
		fmt.Printf("Temujin v%s\n", version)

	default:
		printUsage()
		os.Exit(1)
	}
}

func handleRaidCLI(s *store.FileStore, cmd string, args []string) {
	switch cmd {
	case "create":
		if len(args) < 1 {
			fmt.Println("Usage: temujin raid create <title> [task-id]")
			os.Exit(1)
		}
		title := args[0]
		taskID := ""
		if len(args) > 1 {
			taskID = args[1]
		}
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
		s.Update(func(tasks []raid.Task) []raid.Task {
			return append([]raid.Task{task}, tasks...)
		})
		fmt.Printf("Created %s | %s\n", taskID, title)

	case "launch":
		// Create AND auto-start the OODA loop (moves to Intel immediately)
		if len(args) < 1 {
			fmt.Println("Usage: temujin raid launch <title>")
			fmt.Println("  Creates a raid and immediately starts scouting (Intel state)")
			os.Exit(1)
		}
		title := args[0]
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
		s.Update(func(tasks []raid.Task) []raid.Task {
			return append([]raid.Task{task}, tasks...)
		})
		fmt.Printf("Launched %s | %s\n", taskID, title)
		fmt.Println("The dispatcher will auto-dispatch Tanma for scouting.")
		fmt.Println("Watch progress: temujin serve  (then open http://localhost:7891)")

	case "state":
		if len(args) < 2 {
			fmt.Println("Usage: temujin raid state <task-id> <new-state> [comment]")
			os.Exit(1)
		}
		comment := ""
		if len(args) > 2 {
			comment = strings.Join(args[2:], " ")
		}
		s.Update(func(tasks []raid.Task) []raid.Task {
			for i := range tasks {
				if tasks[i].ID == args[0] {
					tasks[i].State = args[1]
					if org, ok := raid.StateOrgMap[args[1]]; ok {
						tasks[i].Org = org
					}
					if comment != "" {
						tasks[i].Now = comment
					}
					tasks[i].UpdatedAt = raid.NowISO()
					break
				}
			}
			return tasks
		})
		fmt.Printf("State: %s -> %s\n", args[0], args[1])

	case "flow":
		if len(args) < 4 {
			fmt.Println("Usage: temujin raid flow <task-id> <from> <to> <remark>")
			os.Exit(1)
		}
		s.Update(func(tasks []raid.Task) []raid.Task {
			for i := range tasks {
				if tasks[i].ID == args[0] {
					tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
						At: raid.NowISO(), From: args[1], To: args[2], Remark: args[3],
					})
					tasks[i].UpdatedAt = raid.NowISO()
					break
				}
			}
			return tasks
		})
		fmt.Printf("Flow: %s -> %s\n", args[1], args[2])

	case "done":
		if len(args) < 1 {
			fmt.Println("Usage: temujin raid done <task-id> [output] [summary]")
			os.Exit(1)
		}
		output := ""
		summary := "Raid complete"
		if len(args) > 1 {
			output = args[1]
		}
		if len(args) > 2 {
			summary = args[2]
		}
		s.Update(func(tasks []raid.Task) []raid.Task {
			for i := range tasks {
				if tasks[i].ID == args[0] {
					tasks[i].State = raid.StateDone
					tasks[i].Output = output
					tasks[i].Now = summary
					tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
						At: raid.NowISO(), From: tasks[i].Org, To: "Khan",
						Remark: "Raid complete: " + summary,
					})
					tasks[i].UpdatedAt = raid.NowISO()
					break
				}
			}
			return tasks
		})
		fmt.Printf("Done: %s\n", args[0])

	case "retreat":
		if len(args) < 1 {
			fmt.Println("Usage: temujin raid retreat <task-id> [reason]")
			os.Exit(1)
		}
		reason := "Strategic retreat"
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
		s.Update(func(tasks []raid.Task) []raid.Task {
			for i := range tasks {
				if tasks[i].ID == args[0] {
					tasks[i].State = raid.StateMigrate
					tasks[i].Now = "Retreated: " + reason
					tasks[i].FlowLog = append(tasks[i].FlowLog, raid.FlowEntry{
						At: raid.NowISO(), From: tasks[i].Org, To: "Migrate",
						Remark: "Retreat: " + reason,
					})
					tasks[i].UpdatedAt = raid.NowISO()
					break
				}
			}
			return tasks
		})
		fmt.Printf("Retreat: %s\n", args[0])

	case "progress":
		if len(args) < 2 {
			fmt.Println("Usage: temujin raid progress <task-id> <text> [todos-pipe]")
			os.Exit(1)
		}
		todos := ""
		if len(args) > 2 {
			todos = args[2]
		}
		handleProgress(s, args[0], args[1], todos)

	default:
		fmt.Printf("Unknown raid command: %s\n", cmd)
		os.Exit(1)
	}
}

func handleProgress(s *store.FileStore, taskID, text, todosPipe string) {
	s.Update(func(tasks []raid.Task) []raid.Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].Now = text
				entry := raid.ProgressEntry{
					At:    raid.NowISO(),
					Text:  text,
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
	fmt.Printf("Progress: %s | %s\n", taskID, text)
}

func getDataDir() string {
	// Check env first, then default
	if d := os.Getenv("TEMUJIN_DATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".temujin", "data")
}

func printUsage() {
	fmt.Printf(`Temujin v%s - The steppe conquers the court.

Usage:
  temujin serve [port]              Start the war room + dispatcher (default: 7891)
  temujin raid launch <title>       Launch raid with auto OODA loop
  temujin raid create <title>       Create raid (manual, stays in Inbox)
  temujin raid state <id> <state>   Update raid state
  temujin raid flow <id> <f> <t> <r> Add flow record
  temujin raid done <id> [out] [sum] Complete a raid
  temujin raid retreat <id> [reason] Retreat from a raid
  temujin yam <id> <text> [todos]   Report progress (yam = messenger)
  temujin version                   Show version

States: Inbox -> Intel -> Kurultai -> March -> Charge -> Yam -> Loot -> Done
        Any state -> Migrate (retreat) | Blocked (stuck)
`, version)
}
