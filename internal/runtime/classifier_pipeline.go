package runtime

import (
	"context"
	"fmt"
	"os"

	"agentx/internal/executor"
	"agentx/internal/llm/fanout"
	"agentx/internal/llm/invoke"
	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/prompting/reconcile"
	"agentx/internal/prompting/task"
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

// maybeEmitTask runs the classifier pipeline over the session's prior turns and this
// turn, publishing a task_proposed event when the turn is actionable. It is a
// best-effort observer: it runs only on a cleanly completed, recorded turn, and any
// classification error yields no task and never disturbs the prompt cycle. It is
// called before recordTurn, so the digest reflects prior history (not this turn,
// which is passed explicitly).
func (o *Orchestrator) maybeEmitTask(ctx context.Context, cycleErr error, record bool, userOrd uint64, turn, response string) {
	if !record || cycleErr != nil {
		return
	}
	o.mu.Lock()
	p := o.taskPipeline
	events := o.historyEvents()
	o.mu.Unlock()
	if p == nil {
		return
	}
	res, err := p.Classify(ctx, events, turn)
	if err != nil {
		return
	}
	rec, ok := task.FromAction(fmt.Sprintf("task-%d", userOrd), turn, res.Action, res.Escalated)
	if !ok {
		return
	}
	o.publish("TASK_PROPOSED", state.ContentTaskProposed, rec)

	// Reconcile and drain. The response classifier [C] is deferred, so the response
	// signal is empty: an actionable task reconciles to Redispatch, an abstained one
	// to Ask. Execution runs only on the non-tool path (the single_tool cycle returns
	// earlier in runPrompt), so it never double-executes.
	o.mu.Lock()
	ex := o.taskExec
	o.mu.Unlock()
	if ex == nil {
		return
	}
	// [C] classifies the model's own response, so the reconciler distinguishes the
	// vivid-willow reify case (model produced the artifact in prose) from a plain
	// redispatch. On this non-tool path no tool actually ran, so a "produced" or
	// narrated "executed" verdict is treated as produced (not proof of execution).
	respSig := reconcile.ResponseSignal{}
	if rd, ok := p.ClassifyResponse(ctx, response); ok {
		respSig = responseSignal(rd)
	}
	route, _ := reconcile.Reconcile(
		reconcile.TurnSignal{Actionable: rec.Status == task.Proposed, Abstained: rec.Status == task.Abstained},
		respSig,
	)
	if route != reconcile.Reify && route != reconcile.Redispatch {
		return // Ask / None / Confirm / Verify: nothing to drain here
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
}

// responseSignal maps a [C] response-classifier verdict to a reconciler signal.
// On the conversational (non-tool) path no tool actually ran, so both "produced"
// and a narrated "executed" mean the model only showed or claimed the action —
// proof of a real effect is absent, so Executed is never set here.
func responseSignal(d fanout.Decision) reconcile.ResponseSignal {
	return reconcile.ResponseSignal{
		Produced:  d.Verdict == "produced" || d.Verdict == "executed",
		Abstained: d.Abstained,
	}
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
