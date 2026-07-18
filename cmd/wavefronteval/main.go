// Command wavefronteval is ADR 0012 Phase 9's comparison harness: it replays a
// fixed goal corpus through both the continuous engine (ADR 0008,
// scheduler.Scheduler + decompose.Decomposer) and wavefront (ADR 0012,
// wavefront.Scheduler) against a real Ollama host, side by side, and reports what
// each engine actually did — wall time, dispatch count, peak concurrency, and the
// root goal's final outcome.
//
// Like cmd/ollamabench, it constructs its own collaborators directly rather than
// going through runtime.Orchestrator: the orchestrator's engine selection is a
// single WavefrontEnabled switch, and this tool needs to run both engines on the
// same goal, which that switch structurally doesn't support.
//
// Tool execution is never real: a recordingExecutor logs each proposed call and
// returns a placeholder success outcome without touching the filesystem or
// network, so this is safe to run repeatedly against a real project directory.
// Both engines' prompts see the same real, read-only tool catalog
// (tools.DefaultRegistry().Available(true)) so the comparison reflects genuine
// grounding behavior, not a stubbed one.
//
// Usage:
//
//	go run ./cmd/wavefronteval -goals ./goals.txt
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agentx/internal/executor"
	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/runtime/wavefront"
	"agentx/internal/session"
	"agentx/internal/tools"
)

// defaultGoals mirrors ADR 0008's own canonical review fixture in spirit — a
// handful of realistic, project-investigation-shaped questions, not toy prompts.
var defaultGoals = []string{
	"What language is this project written in and what does it do?",
	"Review the current project and identify one under-developed feature.",
	"Does this project have a test suite, and what does it cover?",
}

func main() {
	var (
		host     = flag.String("host", "localhost:11434", "Ollama host:port")
		model    = flag.String("model", "nemotron-cascade-2:latest", "model name (must be installed)")
		goalsF   = flag.String("goals", "", "path to a newline-separated goal corpus file (default: a small built-in corpus)")
		slots    = flag.Int("slots", decompose.DefaultSlots, "concurrent dispatch budget, shared by both engines")
		maxDepth = flag.Int("maxdepth", decompose.DefaultMaxDepth, "recursion depth cap, shared by both engines")
		timeout  = flag.Duration("timeout", 3*time.Minute, "per-engine-per-goal timeout")
	)
	flag.Parse()

	goals := defaultGoals
	if *goalsF != "" {
		data, err := os.ReadFile(*goalsF)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read goals file: %v\n", err)
			os.Exit(2)
		}
		goals = nil
		for line := range strings.Lines(string(data)) {
			if g := strings.TrimSpace(line); g != "" {
				goals = append(goals, g)
			}
		}
	}

	client := ollama.New(*host)
	readyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := client.Ready(readyCtx, *model)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ollama not ready: %v\n", err)
		os.Exit(1)
	}

	catalog := renderCatalog(tools.DefaultRegistry())
	chat := newCompleteChat(client, *model)

	dec := decompose.Decomposer{
		Planner:   decompose.LLMPlanner{Chat: decompose.Chat(chat), Catalog: catalog},
		SessionID: "wavefronteval",
		MaxDepth:  branch.DefaultMaxDepth,
		Facts:     func() []session.Fact { return nil },
	}
	classifier := wavefront.LLMClassifier{Chat: chat, Catalog: catalog}

	fmt.Fprintf(os.Stderr, "# host=%s model=%s slots=%d maxdepth=%d goals=%d\n", *host, *model, *slots, *maxDepth, len(goals))
	printHeader()

	for i, goal := range goals {
		rootID := fmt.Sprintf("eval-%d", i+1)
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		cont := runContinuous(ctx, rootID, goal, dec, *slots, *maxDepth)
		cancel()

		ctx, cancel = context.WithTimeout(context.Background(), *timeout)
		wf := runWavefront(ctx, rootID+"-wf", goal, classifier, chat, *slots, *maxDepth)
		cancel()

		printGoal(goal, cont, wf)
	}
}

// engineResult is one engine's outcome for one goal — deliberately the same shape
// for both engines, since scheduler.Scheduler and wavefront.Scheduler already
// expose DispatchOrder()/Peak() symmetrically (ADR 0012 Phase 7).
type engineResult struct {
	wall         time.Duration
	dispatches   int
	peak         int
	nodes        int
	status       task.Status
	value        string
	errText      string
	schedulerErr error
}

func runContinuous(ctx context.Context, rootID, goal string, dec decompose.Decomposer, slots, maxDepth int) engineResult {
	root := task.Record{ID: rootID, Goal: goal, Type: task.Query, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}}
	g := task.NewGraph()
	if err := g.Add(root); err != nil {
		return engineResult{schedulerErr: err}
	}
	sch := scheduler.New(g, dec, recordingExecutor{}, slots, maxDepth)
	start := time.Now()
	err := sch.Run(ctx)
	wall := time.Since(start)

	rec, _ := g.Node(rootID)
	return engineResult{
		wall: wall, dispatches: len(sch.DispatchOrder()), peak: sch.Peak(), nodes: g.Len(),
		status: rec.Status, value: rec.Value, errText: rec.Error, schedulerErr: err,
	}
}

func runWavefront(ctx context.Context, rootID, goal string, classifier wavefront.Classifier, chat wavefront.Chat, slots, maxDepth int) engineResult {
	root := task.Record{ID: rootID, Goal: goal, Type: task.Query, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}}
	g := task.NewGraph()
	if err := g.Add(root); err != nil {
		return engineResult{schedulerErr: err}
	}
	sch := wavefront.New(g, goal, classifier, chat, "", recordingExecutor{}, slots, maxDepth)
	start := time.Now()
	err := sch.Run(ctx)
	wall := time.Since(start)

	rec, _ := g.Node(rootID)
	return engineResult{
		wall: wall, dispatches: len(sch.DispatchOrder()), peak: sch.Peak(), nodes: g.Len(),
		status: rec.Status, value: rec.Value, errText: rec.Error, schedulerErr: err,
	}
}

// recordingExecutor logs each proposed call and returns a placeholder success
// outcome without ever touching the filesystem or network — this harness measures
// decomposition behavior, not tool execution (already covered by the existing
// executor/tools test suites), so it stays safe to run repeatedly.
type recordingExecutor struct{}

func (recordingExecutor) Execute(ctx context.Context, rec task.Record) executor.Outcome {
	tool, _ := rec.Params["tool"].(string)
	args, _ := rec.Params["args"].(map[string]any)
	return executor.Outcome{
		Status: executor.Executed,
		Result: tools.Result{Preview: fmt.Sprintf("[recorded, not run] %s %v", tool, args)},
	}
}

// newCompleteChat mirrors internal/runtime's helper of the same name (not
// reusable directly — that one is unexported in the runtime package, and this
// tool deliberately does not depend on internal/runtime at all).
func newCompleteChat(client *ollama.Client, model string) wavefront.Chat {
	return func(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error) {
		return client.Complete(ctx, ollama.CompleteRequest{
			Model: model,
			Messages: []ollama.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
			Format: format,
		})
	}
}

// renderCatalog mirrors internal/runtime's plannerCatalog for the same reason
// newCompleteChat does — read-only tools only, matching ADR 0008's investigating-
// branch convention (this tool never executes anything for real, but a read-only
// catalog keeps the comparison representative of what a real investigation phase
// would see).
func renderCatalog(reg *tools.Registry) string {
	if reg == nil {
		return "(no tools available)"
	}
	var b strings.Builder
	for _, d := range reg.Available(true) {
		names := make([]string, len(d.Args))
		for i, a := range d.Args {
			names[i] = a.Name
		}
		fmt.Fprintf(&b, "- %s: args {%s} (%s)\n", d.ID, strings.Join(names, ", "), d.Risk)
	}
	return b.String()
}

func printHeader() {
	fmt.Printf("%-60s %-12s %-9s %-6s %-4s %-5s %-10s %s\n",
		"goal", "engine", "wall", "dispatch", "peak", "nodes", "status", "value/error")
	fmt.Println(strings.Repeat("-", 140))
}

func printGoal(goal string, cont, wf engineResult) {
	printRow(goal, "continuous", cont)
	printRow("", "wavefront", wf)
}

func printRow(goal, engine string, r engineResult) {
	label := truncate(goal, 58)
	outcome := string(r.status)
	detail := r.value
	if r.errText != "" {
		detail = r.errText
	}
	if r.schedulerErr != nil {
		outcome = "scheduler-error"
		detail = r.schedulerErr.Error()
	}
	fmt.Printf("%-60s %-12s %-9s %-6d %-4d %-5d %-10s %s\n",
		label, engine, ms(r.wall), r.dispatches, r.peak, r.nodes, outcome, truncate(detail, 60))
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func ms(d time.Duration) string { return fmt.Sprintf("%dms", d.Milliseconds()) }
