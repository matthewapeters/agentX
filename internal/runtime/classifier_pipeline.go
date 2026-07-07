package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"agentx/internal/executor"
	"agentx/internal/llm/fanout"
	"agentx/internal/llm/invoke"
	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/prompting/reconcile"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/session"
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// taskExecutor drains a proposed task record into a verified effect. The concrete
// implementation is *executor.Executor; the interface lets tests inject a stub.
type taskExecutor interface {
	Execute(ctx context.Context, rec task.Record) executor.Outcome
}

// digestMaxTurns bounds the v1 session digest window fed to the classifier.
const digestMaxTurns = 12

// WithTaskClassifier injects the classifier pipeline (for tests). Without it, the
// orchestrator builds a live Ollama-backed pipeline at Start when a prompt corpus is
// configured (Settings.PromptCorpus).
func WithTaskClassifier(p *pipeline.Pipeline) Option {
	return func(o *Orchestrator) { o.taskPipeline = p }
}

// WithTaskExecutor injects the task executor (for tests). Without it, the
// orchestrator builds one from its tool collaborators at Start when the classifier
// is active and tools are enabled.
func WithTaskExecutor(e taskExecutor) Option {
	return func(o *Orchestrator) { o.taskExec = e }
}

// WithDecomposition injects the atomicity oracle and decomposer that drive the
// Decompose route (ADR 0008 Phase 4). Without them, a Decompose-routed turn records the
// route as a diagnostic but does not run a plan (the decomposer is not yet wired). The
// real oracle/decomposer are built from the classifier + LLM planner in Phase 4e.
func WithDecomposition(oracle scheduler.Oracle, decomposer scheduler.Decomposer) Option {
	return func(o *Orchestrator) {
		o.taskOracle = oracle
		o.taskDecomp = decomposer
	}
}

// buildTaskClassifier constructs a live pipeline from settings when a prompt corpus
// is configured and none was injected. The feature is presence-gated and default
// off: with no corpus the orchestrator's prompt cycle is unchanged. A malformed
// corpus simply leaves the classifier off rather than failing startup. Caller holds
// o.mu.
func (o *Orchestrator) buildTaskClassifier() {
	if o.taskPipeline != nil || o.settings.PromptCorpus == "" {
		return
	}
	c, err := corpus.Parse([]byte(o.settings.PromptCorpus))
	if err != nil {
		return // opt-in feature: a bad corpus stays off, prompt cycle unaffected
	}
	client := ollama.New(o.settings.OllamaHost)
	inv := invoke.NewOllama(client, o.settings.OllamaModel, "")
	runner := cascade.NewRunner(inv, fanout.WithServerDefaults())
	o.taskPipeline = pipeline.New(runner, c, digestMaxTurns)
}

// buildTaskExecutor wires the classifier's tasks to a real, verified execution
// path, reusing the command stack. It builds only when the classifier is active
// and tools are enabled (and none was injected); otherwise the classifier stays a
// pure observer that emits task_proposed without executing. Caller holds o.mu.
func (o *Orchestrator) buildTaskExecutor() {
	if o.taskExec != nil || o.taskPipeline == nil || !o.toolsReady() {
		return
	}
	// Confine execution to the working directory; a task that would operate outside
	// it prompts the user through the existing interactive approval gate.
	root, _ := os.Getwd()
	approver := executor.ApproverFunc(func(ctx context.Context, d tools.Descriptor, args map[string]string, _ string) bool {
		v, err := o.RequestApproval(ctx, d, args, o.policy)
		return err == nil && v.Decision == tools.Allow
	})
	o.taskExec = executor.New(
		o.proposer, o.registry, o.policy, o.runner,
		executor.FSVerifier{Root: root},
		executor.WithRoot(root),
		executor.WithApprover(approver),
	)
}

// buildDecomposition wires the Decompose route's atomicity oracle and branch-backed
// decomposer, turning the compound-goal path on in production. It needs the classifier
// (the oracle runs action_classify) and the executor (leaves drain through it), and is a
// no-op if either is absent or if a decomposer was injected (tests). The planner calls the
// same Ollama model as the classifier — one model, prompt-engineered. Caller holds o.mu.
func (o *Orchestrator) buildDecomposition() {
	if o.taskDecomp != nil || o.taskPipeline == nil || o.taskExec == nil {
		return
	}
	o.taskOracle = decompose.Oracle{
		Action:  decompose.PipelineActions{P: o.taskPipeline},
		OneStep: decompose.HeuristicOneStep{},
	}
	client := ollama.New(o.settings.OllamaHost)
	model := o.settings.OllamaModel
	chat := func(ctx context.Context, prompt string) (string, error) {
		return client.Complete(ctx, ollama.CompleteRequest{
			Model:    model,
			Messages: []ollama.Message{{Role: "user", Content: prompt}},
		})
	}
	sessionID := o.id.ID
	o.taskDecomp = decompose.Decomposer{
		Planner:   decompose.LLMPlanner{Chat: chat},
		SessionID: sessionID,
		MaxDepth:  branch.DefaultMaxDepth,
		Facts: func() []session.Fact {
			wm, err := o.store.LoadWorkingMemory(sessionID)
			if err != nil {
				return nil
			}
			return wm.Enabled()
		},
	}
}

// maybeEmitTask runs the classifier over this turn and folds it with a response
// classification ([C], always-on) into a route, then acts on it. It is a
// best-effort observer: it runs only on a recorded turn, any classification error
// yields no task, and it never disturbs the prompt cycle. Every exit past the
// disabled-classifier guard publishes a task_diagnostic (the three stage scores +
// an outcome/skip reason), so no turn is silently dropped. It runs before
// recordTurn, so the digest reflects prior history (this turn is passed
// explicitly), and only on runPrompt's non-tool path, so it never double-executes.
func (o *Orchestrator) maybeEmitTask(ctx context.Context, cycleErr error, record bool, userOrd uint64, turn, response string) {
	if !record {
		return // non-recorded (bootstrap) turn: not a user task
	}
	o.mu.Lock()
	p := o.taskPipeline
	events := o.historyEvents()
	ex := o.taskExec
	o.mu.Unlock()
	if p == nil {
		return // task classifier disabled (no corpus): nothing to diagnose
	}
	// Every exit below publishes a task_diagnostic, so a skipped turn always leaves a
	// legible reason (and the three stage scores, once classification has run).
	if cycleErr != nil {
		o.publishTaskDiag(nil, nil, "skipped", fmt.Sprintf("response cycle error: %v", cycleErr))
		return
	}
	res, err := p.Classify(ctx, events, turn)
	if err != nil {
		o.publishTaskDiag(nil, nil, "skipped", fmt.Sprintf("classify error: %v", err))
		return
	}

	// [C] runs on every turn, independent of [B], so it catches an action the model
	// volunteered on a turn the user did not ask for one. On this non-tool path no
	// tool actually ran, so "produced"/narrated "executed" are treated as produced.
	var respDecision *fanout.Decision
	respSig := reconcile.ResponseSignal{}
	if rd, ok := p.ClassifyResponse(ctx, response); ok {
		respDecision = &rd
		respSig = responseSignal(rd)
	}
	rec, hasTask := task.FromAction(fmt.Sprintf("task-%d", userOrd), turn, res.Action, res.Escalated)
	route, why := reconcile.Reconcile(
		reconcile.TurnSignal{
			Actionable: hasTask && rec.Status == task.Proposed,
			Abstained:  hasTask && rec.Status == task.Abstained,
		},
		respSig,
	)

	switch route {
	case reconcile.Confirm:
		// A volunteered action — the model did or produced something the user never
		// requested. Never auto-execute it; surface it for approval.
		o.publish("TASK_RESULT", state.ContentTaskResult, map[string]any{
			"route":  string(route),
			"status": string(executor.NeedsApproval),
			"reason": "volunteered action — approval required",
		})
		o.publishTaskDiag(&res, respDecision, string(executor.NeedsApproval), fmt.Sprintf("%s: %s", route, why))
		return
	case reconcile.Reify, reconcile.Redispatch, reconcile.Verify:
		// A requested action: emit the durable task and drain it.
	case reconcile.Decompose:
		// A compound goal: the model narrated a multi-step action. When the oracle and
		// decomposer are wired (Phase 4e), run the plan through the scheduler in the
		// background so the turn is not blocked; otherwise record the route so the decision
		// is legible rather than mislabeled "uncertain".
		if o.taskOracle != nil && o.taskDecomp != nil && ex != nil && hasTask {
			o.publishTaskDiag(&res, respDecision, "decompose", fmt.Sprintf("%s: %s", route, why))
			go o.runDecomposition(context.Background(), rec, ex)
			return
		}
		o.publishTaskDiag(&res, respDecision, "decompose", fmt.Sprintf("%s (decomposer not wired): %s", route, why))
		return
	default:
		o.publishTaskDiag(&res, respDecision, "skipped", fmt.Sprintf("reconciled to %s: %s", route, why))
		return // None (pure conversation) / Ask (abstained)
	}
	if !hasTask {
		o.publishTaskDiag(&res, respDecision, "skipped", fmt.Sprintf("%s but action produced no task record", route))
		return
	}
	o.publish("TASK_PROPOSED", state.ContentTaskProposed, rec)
	if ex == nil {
		o.publishTaskDiag(&res, respDecision, "skipped", "executor unavailable (tools disabled)")
		return
	}
	outcome := ex.Execute(ctx, rec)
	o.publish("TASK_RESULT", state.ContentTaskResult, map[string]any{
		"task_id":  rec.ID,
		"route":    string(route),
		"status":   string(outcome.Status),
		"tool":     outcome.Result.ToolID,
		"verified": outcome.Status == executor.Executed,
		"reason":   outcome.Reason,
	})
	o.publishTaskDiag(&res, respDecision, string(outcome.Status), fmt.Sprintf("%s → %s", route, outcome.Status))
}

// runDecomposition drains a compound goal through the scheduler in the background (so the
// turn is never blocked), streaming per-node task_node deltas as it goes (ADR 0009 §9a —
// all execution is user-visible while it runs) and closing with the final task_plan
// snapshot. It reuses the tuned classifier (via the oracle) and a read-restricted branch
// (via the decomposer). Best-effort, like the rest of the classifier path: a decomposition
// error is surfaced on the plan event, never as a disturbed prompt cycle.
func (o *Orchestrator) runDecomposition(ctx context.Context, root task.Record, ex taskExecutor) {
	outcome, err := decompose.DrainPlan(ctx, root, o.taskOracle, o.taskDecomp, ex,
		decompose.DefaultSlots, decompose.DefaultMaxDepth, &planObserver{o: o, root: root.ID})

	executed := 0
	nodes := make([]map[string]any, 0, len(outcome.Nodes))
	for _, n := range outcome.Nodes {
		nodes = append(nodes, map[string]any{
			"task_id": n.ID, "goal": n.Goal, "status": string(n.Status), "deps": n.Deps,
		})
		if n.Status == task.Done {
			executed++
		}
	}
	payload := map[string]any{
		"root": root.ID, "goal": root.Goal, "phase": "ended", "nodes": nodes,
		"executed": executed,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	o.publish("TASK_PLAN", state.ContentTaskPlan, payload)
}

// responseSignal maps a [C] response-classifier verdict to a reconciler signal.
// On the conversational (non-tool) path no tool actually ran, so both "produced"
// and a narrated "executed" mean the model only showed or claimed the action —
// proof of a real effect is absent, so Executed is never set here.
func responseSignal(d fanout.Decision) reconcile.ResponseSignal {
	// When the response vote abstained, its spread shape discriminates a compound action
	// (scattered toward produced/executed → the model narrated multiple steps) from
	// genuine ambiguity (weighted toward "none"/"neither"). action > inaction leans
	// produced, routing the fold to Decompose rather than Ask.
	action := d.Spread["produced"] + d.Spread["executed"]
	inaction := d.Spread["none"] + d.Spread["neither"]
	return reconcile.ResponseSignal{
		Produced:      d.Verdict == "produced" || d.Verdict == "executed",
		Abstained:     d.Abstained,
		LeansProduced: action > inaction,
	}
}

// publishTaskDiag publishes a task_diagnostic event: the three fan-group stage
// verdicts (triage + action from res, response from resp — any nil when that stage
// did not run), the turn's outcome ("skipped"/an executor status), and a reason.
// Observability only; every maybeEmitTask exit past the disabled-classifier guard
// calls it, so a skipped turn is never silent.
func (o *Orchestrator) publishTaskDiag(res *pipeline.Result, resp *fanout.Decision, outcome, reason string) {
	payload := map[string]any{
		"outcome": outcome,
		"reason":  reason,
		"text":    taskDiagText(res, resp, outcome, reason),
	}
	if res != nil {
		payload["triage"] = decisionMap(res.Triage)
		payload["action"] = decisionMap(res.Action)
		payload["escalated"] = res.Escalated
	}
	if resp != nil {
		payload["response"] = decisionMap(*resp)
	}
	o.publish("TASK_DIAGNOSTIC", state.ContentTaskDiagnostic, payload)
}

// taskDiagText renders the human-readable diagnostic body: one line per stage that
// ran, then the outcome and reason.
func taskDiagText(res *pipeline.Result, resp *fanout.Decision, outcome, reason string) string {
	var lines []string
	if res != nil {
		lines = append(lines, formatDecision("triage", res.Triage), formatDecision("action", res.Action))
	}
	if resp != nil {
		lines = append(lines, formatDecision("response", *resp))
	}
	head := outcome
	if reason != "" {
		head += " · " + reason
	}
	return strings.Join(append(lines, head), "\n")
}

// decisionMap projects a fan-out decision into the durable event payload.
func decisionMap(d fanout.Decision) map[string]any {
	return map[string]any{
		"verdict":    d.Verdict,
		"confidence": d.Confidence,
		"abstained":  d.Abstained,
		"spread":     d.Spread,
	}
}

// formatDecision renders one stage verdict for the widget:
// "<label>: <verdict|abstained> [(conf)] [{spread}]".
func formatDecision(label string, d fanout.Decision) string {
	v := d.Verdict
	if d.Abstained {
		v = "abstained"
	}
	if v == "" {
		v = "—"
	}
	s := label + ": " + v
	if !d.Abstained && d.Confidence > 0 {
		s += fmt.Sprintf(" (%.0f%%)", d.Confidence*100)
	}
	if len(d.Spread) > 0 {
		s += " " + formatSpread(d.Spread)
	}
	return s
}

// formatSpread renders a vote spread deterministically, highest-voted first:
// "{artifact:2 · command:1}".
func formatSpread(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, m[k]))
	}
	return "{" + strings.Join(parts, " · ") + "}"
}

// historyEvents projects the in-memory conversation history into events for the
// digest builder, preserving each turn's enabled state and ordinal. Caller holds
// o.mu.
func (o *Orchestrator) historyEvents() []state.Event {
	events := make([]state.Event, 0, len(o.history))
	for _, h := range o.history {
		ct := state.ContentUserPrompt
		if h.role == "assistant" {
			ct = state.ContentAgentResponse
		}
		events = append(events, state.Event{
			ContentType: ct,
			Payload:     map[string]any{"text": h.content},
			Enabled:     h.enabled,
			Ordinal:     h.ordinal,
		})
	}
	return events
}
