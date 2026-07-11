package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/session"
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

// WithDecomposition injects the decomposer that drives the Decompose route (ADR 0008,
// amended 2026-07-07: the planner declares each child's Kind at generation time, so no
// separate atomicity oracle is wired here). Without it, a Decompose-routed turn records the
// route as a diagnostic but does not run a plan.
func WithDecomposition(decomposer scheduler.Decomposer) Option {
	return func(o *Orchestrator) { o.taskDecomp = decomposer }
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
		executor.WithCallObserver(taskToolPublisher{o: o}),
	)
}

// taskToolPublisher streams executor call lifecycle to the bus, tagged with the owning
// task id so the output surface nests the 🔧 call and 📋 result under the plan step that
// ran them (ADR 0009 §9c). Untagged tool events (the single_tool cycle) render flat.
type taskToolPublisher struct{ o *Orchestrator }

func (p taskToolPublisher) ToolCalled(rec task.Record, d tools.Descriptor, args map[string]string) {
	p.o.bus.Publish(state.Event{
		Epoch: time.Now().UnixMilli(), SessionID: p.o.id.ID,
		EventType: "TOOL_CALL", ContentType: state.ContentToolCall, ToolName: d.ID,
		Payload:   map[string]any{"text": proposalText(d, args), "task_id": rec.ID},
		ModelName: p.o.settings.OllamaModel,
	})
	p.o.planTrees.toolCalled(rec, d, args, p.o.store, p.o.id.ID)
}

func (p taskToolPublisher) ToolFinished(rec task.Record, d tools.Descriptor, res tools.Result, status executor.Status) {
	if res.ToolID == "" {
		res.ToolID = d.ID // a call that never ran still reports which tool it was
	}
	p.o.bus.Publish(state.Event{
		Epoch: time.Now().UnixMilli(), SessionID: p.o.id.ID,
		EventType: "TOOL_RESULT", ContentType: state.ContentToolResult, ToolName: res.ToolID,
		Payload: map[string]any{
			"text": toolResultText(res), "task_id": rec.ID, "outcome": string(status),
			"status": res.Status, "exit": res.Exit, "ref": res.Ref, "bytes": res.Bytes,
		},
		ModelName: p.o.settings.OllamaModel,
	})
	p.o.planTrees.toolFinished(rec, res, status, p.o.store, p.o.id.ID)
}

// buildDecomposition wires the Decompose route's branch-backed decomposer, turning the
// compound-goal path on in production. It needs only the executor (leaves drain through
// it) — unlike before the ADR 0008 amendment, it does NOT need the experimental prompt
// corpus (o.taskPipeline): the planner declares each child's Kind itself, so there is no
// atomicity oracle here to build from the classifier. This is a deliberate behavior
// broadening: invoke_planner now works even with no prompts.toml configured. No-op if the
// executor is absent or a decomposer was injected (tests). The planner calls the same
// Ollama model as the classifier — one model, prompt-engineered. Caller holds o.mu.
func (o *Orchestrator) buildDecomposition() {
	if o.taskDecomp != nil || o.taskExec == nil {
		return
	}
	client := ollama.New(o.settings.OllamaHost)
	model := o.settings.OllamaModel
	chat := func(ctx context.Context, prompt string, format json.RawMessage) (string, error) {
		return client.Complete(ctx, ollama.CompleteRequest{
			Model:    model,
			Messages: []ollama.Message{{Role: "user", Content: prompt}},
			Format:   format,
		})
	}
	sessionID := o.id.ID
	o.taskDecomp = decompose.Decomposer{
		Planner: decompose.LLMPlanner{
			Chat:     chat,
			Template: o.settings.PlannerPrompt,
			Catalog:  plannerCatalog(o.registry),
		},
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

// plannerCatalog renders a compact, drift-proof tool list for the planner prompt: id +
// arg names + risk tier, built from the live registry (never hand-duplicated text, so it
// cannot drift from what's actually callable). Deliberately terser than
// agentx-shell-commands.md's markdown catalog — that file also carries the proposer's
// "how to reply" framing, which would conflict with the planner's own reply-shape
// instructions; a nil registry (tools disabled) renders an empty catalog.
func plannerCatalog(reg *tools.Registry) string {
	if reg == nil {
		return "(no tools available)"
	}
	var b strings.Builder
	for _, d := range reg.Available(false) {
		names := make([]string, len(d.Args))
		for i, a := range d.Args {
			names[i] = a.Name
		}
		fmt.Fprintf(&b, "- %s: args {%s} (%s)\n", d.ID, strings.Join(names, ", "), d.Risk)
	}
	return b.String()
}

// askClarifyOptions is the fixed two-way choice offered when the classifier itself is
// genuinely unsure whether a turn requested an action (reconcile.Ask) but produced a
// concrete task record anyway — confirm before guessing, never silently drop it.
var askClarifyOptions = []state.ApprovalOption{
	{Label: "Yes, do it", Decision: "yes"},
	{Label: "No, just chatting", Decision: "no"},
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
			Actionable:      hasTask && rec.Status == task.Proposed,
			Abstained:       hasTask && rec.Status == task.Abstained,
			LeansActionable: leansActionable(res.Action),
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
		// A compound goal: the model narrated a multi-step action. When the decomposer is
		// wired, run the plan through the scheduler in the background so the turn is not
		// blocked; otherwise record the route so the decision is legible rather than
		// mislabeled "uncertain". The reconciler has already judged this goal multi-step,
		// so the root is constructed as a Step directly (ADR 0008 amendment) — no oracle
		// re-litigates that.
		if o.taskDecomp != nil && ex != nil && hasTask {
			rec.Kind = task.KindStep
			o.publishTaskDiag(&res, respDecision, "decompose", fmt.Sprintf("%s: %s", route, why))
			go o.runDecomposition(context.Background(), rec, ex)
			return
		}
		o.publishTaskDiag(&res, respDecision, "decompose", fmt.Sprintf("%s (decomposer not wired): %s", route, why))
		return
	default:
		// None (pure conversation) always stays silent — there is nothing to confirm. Ask
		// (genuine classifier ambiguity) used to stay silent too, with no visible follow-up
		// (reconcile.go: "which today has no visible follow-up — clever-raven-3") — dead
		// end #3 alongside the plan-incomplete cases above. When there is a concrete task
		// record to ask about, use the same decision gate tool-approval and verb-
		// continuation already share instead of silently guessing or silently dropping it.
		if route != reconcile.Ask || !hasTask || ex == nil {
			o.publishTaskDiag(&res, respDecision, "skipped", fmt.Sprintf("reconciled to %s: %s", route, why))
			return
		}
		if !o.resolveAskRoute(ctx, rec) {
			o.publishTaskDiag(&res, respDecision, "skipped", fmt.Sprintf("%s: %s (declined)", route, why))
			return
		}
		// Confirmed: fall through to the shared dispatch below, same as Reify/Redispatch/
		// Verify.
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

// resolveAskRoute is maybeEmitTask's Ask-route branch, split out so the decision-gate
// interaction is unit-testable without a live classifier pipeline: ask whether to proceed
// with the task record the reconciler already built, rather than silently guessing or
// silently dropping it. true only on an explicit "yes" with no error (ctx canceled, a
// denial, or any other decision string all mean don't proceed).
func (o *Orchestrator) resolveAskRoute(ctx context.Context, rec task.Record) bool {
	dec, derr := o.RequestDecision(ctx, state.PhaseClarify,
		fmt.Sprintf("Did you want me to %s?", rec.Goal), askClarifyOptions)
	return derr == nil && dec == "yes"
}

// runDecomposition drains a compound goal through the scheduler in the background (so the
// turn is never blocked), streaming per-node task_node deltas as it goes (ADR 0009 §9a —
// all execution is user-visible while it runs), then — unlike before — closes the loop with
// a synthesized follow-up answer when it found anything, mirroring what the foreground plan
// cycle (runPlanPhase) already does. Without this, a background Decompose ran real work and
// then went silent: the model's own turn had already finished, so nothing ever told the
// user what the investigation found (clever-raven-3 — the agent narrated "let me check...
// and run it now" and then just stopped). root already carries Kind: task.KindStep (set by
// the caller — the reconciler judged this goal compound, so there is no oracle to
// re-litigate that here). Best-effort: a decomposition error is surfaced on the plan event,
// never as a disturbed prompt cycle.
func (o *Orchestrator) runDecomposition(ctx context.Context, root task.Record, ex taskExecutor) {
	o.setProcessing(state.StateWorking, state.PhasePlanning)

	cap := &capturingExec{inner: ex, reader: o.artifactStore()}
	ctx = withComposedFindings(ctx, cap)
	outcome, err := decompose.DrainPlan(ctx, root, o.taskDecomp, cap,
		decompose.DefaultSlots, decompose.DefaultMaxDepth, &planObserver{o: o, root: root.ID})

	executed := 0
	for _, n := range outcome.Nodes {
		if n.Status == task.Done {
			executed++
		}
	}
	o.publish("TASK_PLAN", state.ContentTaskPlan, planSummary(root, outcome.Nodes, executed, err))

	proceed, note, cerr := o.confirmPlanIncomplete(ctx, root, outcome.Nodes)
	if cerr != nil {
		// Interrupted while awaiting the clarify decision: end the background cycle
		// cleanly, same as any other ctx cancellation here.
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return
	}
	if !proceed {
		msgs := o.withContext(o.assembler.Assemble(root.Goal + planStoppedContext(root)))
		resp, respOrd, rerr := o.streamResponse(ctx, msgs, nil, false, false)
		o.recordTurn(rerr, true, 0, "", respOrd, resp)
		o.finishCycle(rerr)
		return
	}

	planCtx := planContext(cap.steps) + note
	if strings.TrimSpace(planCtx) == "" {
		// Nothing investigated successfully: no grounded follow-up to add. The task_plan's
		// own "incomplete" error (if any) is the visible signal; close out the background
		// cycle so the surface doesn't sit on "working" forever.
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return
	}
	msgs := o.withContext(o.assembler.Assemble(root.Goal + planCtx))
	resp, respOrd, rerr := o.streamResponse(ctx, msgs, nil, false, false)
	o.recordTurn(rerr, true, 0, "", respOrd, resp)
	o.finishCycle(rerr)
}

// leansActionable discriminates an abstained [B] action vote whose spread scattered across
// actionable labels from one that leans toward "none" (genuine non-action ambiguity).
// Structural, not enum-exhaustive: the strict contract enum is artifact/command/query/none,
// but a hallucinated out-of-enum label (still quarantined at the fold, not fixed here) is
// still clearly action-flavored — everything except "none" counts toward action, so this
// doesn't need to enumerate valid task_types to work correctly (clever-raven-3: spread
// {command:1, investigation/structure_review:1}, zero weight on "none" → leans actionable).
func leansActionable(d fanout.Decision) bool {
	action := 0
	for label, n := range d.Spread {
		if label != "none" {
			action += n
		}
	}
	return action > d.Spread["none"]
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
