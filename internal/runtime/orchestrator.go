package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	"agentx/internal/tools"
	transporthttp "agentx/internal/transport/http"
)

// Orchestrator satisfies the transport's Provider seam (read + write surface).
var _ transporthttp.Provider = (*Orchestrator)(nil)

// Settings are the runtime inputs the composition root derives from configuration.
type Settings struct {
	// SessionRoot is the directory under which sessions are stored.
	SessionRoot string
	// OllamaHost and OllamaModel configure the model adapter (used by the prompt
	// cycle in CHT-C*).
	OllamaHost  string
	OllamaModel string
	// Instructions is the standing user-instructions text prefixed to every LLM
	// context (from ~/.config/agentx/agentx-instructions.md). Empty falls back to
	// the built-in default system prompt.
	Instructions string
	// BootstrapPrompt, when non-empty, is submitted automatically at startup
	// (from ~/.config/agentx/bootstrap-prompt.md).
	BootstrapPrompt string
	// ClassificationPrompt is the system prompt for the classify step (from
	// ~/.config/agentx/agentx-classification.md). Empty uses the built-in default.
	ClassificationPrompt string
	// ClassificationRetries is the classify-cycle retry budget.
	ClassificationRetries int
	// MaxWidgetLines is the output-widget body-row cap surfaced to the chat UI.
	MaxWidgetLines int
	// ThinkingEnabled is the master switch for model reasoning during the respond
	// phase, streamed as thinking events ahead of the answer.
	ThinkingEnabled bool
	// ThinkingPrompt is the guidance folded into the respond system prompt when
	// thinking (from ~/.config/agentx/agentx-thinking.md). Empty uses the default.
	ThinkingPrompt string
	// ThinkingBudget bounds the thinking phase; when it elapses before any content
	// arrives the runtime falls back to a direct (non-thinking) answer. <=0 disables.
	ThinkingBudget time.Duration
	// ThinkingRoutes enables thinking per classification route. A route absent from
	// the map (or an unclassified prompt) does not think.
	ThinkingRoutes map[string]bool
	// ActiveBorderColor and InactiveBorderColor are SGR foreground parameters for
	// the chat surface's focus-aware panel and widget borders (from [agentx.theme]).
	ActiveBorderColor   string
	InactiveBorderColor string
	// ToolsEnabled turns on the single_tool execution cycle.
	ToolsEnabled bool
	// ToolReadOnly restricts execution to read-risk tools (the rollout default).
	ToolReadOnly bool
	// ToolCatalog is the LLM-facing tool catalog injected into the proposal prompt
	// (from agentx-shell-commands.md). Empty uses tools.DefaultCatalog.
	ToolCatalog string
	// ToolBlacklistPath and ToolApprovalsPath persist the command policy across
	// sessions (blacklist rules in, global approvals in/out). Empty disables I/O.
	ToolBlacklistPath string
	ToolApprovalsPath string
	// ToolOutputMaxBytes caps captured tool output before truncation (full output
	// still persists to the artifact). <=0 uses the executor default.
	ToolOutputMaxBytes int
	// TransportEnabled serves the HTTP/SSE transport alongside the in-process chat
	// so external surfaces can attach. When false, the runtime stays in-process.
	TransportEnabled bool
	// TransportHost is the loopback host the transport binds (e.g. 127.0.0.1).
	TransportHost string
	// TransportPortStart and TransportPortEnd bound the candidate port range the
	// transport binds the first free port from.
	TransportPortStart int
	TransportPortEnd   int
}

// Orchestrator owns the per-process runtime: session, event bus, processing
// state, and persistence.
type Orchestrator struct {
	settings Settings

	store      *session.Store
	id         session.Identity
	bus        *state.Bus
	proc       *state.ProcessingPublisher
	token      surfaces.AttachToken
	surfaceReg *surfaces.Registry
	server     *transporthttp.Server
	endpoint   string
	serveDone  chan error
	model      Model
	assembler  *prompting.Assembler
	classifier *classify.Classifier
	recDone    chan error
	recSub     *state.Subscription
	gate       approvalGate
	registry   *tools.Registry
	policy     *tools.Policy
	proposer   *tools.Proposer
	runner     ToolRunner

	mu        sync.Mutex
	started   bool
	accepting bool
	history   []prompting.Message
}

// Option configures an Orchestrator at construction time.
type Option func(*Orchestrator)

// WithModel overrides the LLM the prompt cycle drives. Without it the
// orchestrator builds a live Ollama adapter from its settings at Start.
func WithModel(m Model) Option {
	return func(o *Orchestrator) { o.model = m }
}

// WithClassifier overrides the prompt classifier. Without it the orchestrator
// builds one from its settings (using the model) at Start.
func WithClassifier(c *classify.Classifier) Option {
	return func(o *Orchestrator) { o.classifier = c }
}

// New returns an unstarted Orchestrator for the given settings.
func New(s Settings, opts ...Option) *Orchestrator {
	o := &Orchestrator{settings: s}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Start runs the startup sequence: create the session, start the bus and
// processing-state feed (idle), and begin draining events to disk.
func (o *Orchestrator) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.started {
		return fmt.Errorf("orchestrator already started")
	}

	o.store = session.NewStore(o.settings.SessionRoot)
	id, err := o.store.Create()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	o.id = id

	// Mint the per-session ephemeral attach token and the surface registry that
	// external surfaces register against (TRN-1). The raw token stays in memory.
	tok, err := surfaces.MintToken()
	if err != nil {
		return fmt.Errorf("mint attach token: %w", err)
	}
	o.token = tok
	o.surfaceReg = surfaces.NewRegistry(tok, id.ID, id.Name)

	if err := o.seedWorkingMemory(); err != nil {
		return fmt.Errorf("seed working memory: %w", err)
	}

	o.bus = state.NewBus()
	o.proc = state.NewProcessingPublisher(id.ID)
	if o.model == nil {
		o.model = newOllamaModel(o.settings.OllamaHost)
	}
	instructions := o.settings.Instructions
	if instructions == "" {
		instructions = prompting.DefaultSystemPrompt
	}
	o.assembler = prompting.New(instructions)
	if o.classifier == nil {
		chat := func(ctx context.Context, msgs []prompting.Message) (string, error) {
			// Classification never thinks (nil onThink): a fast strict-JSON verdict.
			return o.model.Chat(ctx, o.settings.OllamaModel, msgs, func(string) {}, nil)
		}
		o.classifier = classify.New(o.settings.ClassificationPrompt, o.settings.ClassificationRetries, chat)
	}
	if o.settings.ToolsEnabled {
		if err := o.buildTools(); err != nil {
			return err
		}
	}

	recorder := o.store.Recorder(id.ID)
	sub := o.bus.Subscribe()
	o.recSub = sub
	o.recDone = make(chan error, 1)
	go func() { o.recDone <- recorder.Run(sub) }()

	// Serve the HTTP/SSE transport for external surfaces (TRN-6). A bind failure
	// on the required transport blocks startup with a clean error.
	if o.settings.TransportEnabled {
		if err := o.startTransport(); err != nil {
			return err
		}
	}

	o.started = true
	o.accepting = true
	return nil
}

// startTransport allocates a loopback port, publishes the endpoint to session
// metadata, and serves the HTTP transport. The caller holds o.mu.
func (o *Orchestrator) startTransport() error {
	ln, err := transporthttp.Allocate(o.settings.TransportHost, o.settings.TransportPortStart, o.settings.TransportPortEnd)
	if err != nil {
		return fmt.Errorf("allocate transport port: %w", err)
	}
	o.endpoint = transporthttp.Endpoint(ln.Addr())
	if err := o.store.WriteTransport(o.id.ID, session.TransportInfo{SessionID: o.id.ID, Endpoint: o.endpoint}); err != nil {
		_ = ln.Close()
		return fmt.Errorf("publish transport endpoint: %w", err)
	}
	o.server = transporthttp.NewServer(o)
	o.serveDone = make(chan error, 1)
	go func() { o.serveDone <- o.server.Serve(ln) }()
	return nil
}

// Shutdown stops accepting prompts, persists a final processing-state snapshot,
// flushes the recorder, and returns. It respects ctx cancellation while waiting
// for the recorder to drain.
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.mu.Lock()
	if !o.started {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator not started")
	}
	o.accepting = false
	sub := o.recSub
	done := o.recDone
	server := o.server
	o.mu.Unlock()

	// Stop the transport first so no new external requests arrive mid-drain, and
	// mark attached surfaces stopped.
	if server != nil {
		_ = server.Shutdown(ctx)
		o.surfaceReg.StopAll()
	}

	// Persist a final processing-state snapshot before draining.
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "PROCESSING_STATE",
		ContentType: state.ContentProcessingState,
		Payload:     o.proc.Current(),
	})

	sub.Close()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Accepting reports whether the orchestrator is accepting new prompts.
func (o *Orchestrator) Accepting() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.accepting
}

// Settings returns the settings the orchestrator was built with.
func (o *Orchestrator) Settings() Settings { return o.settings }

// Bus returns the canonical event bus (for surfaces and the prompt cycle).
func (o *Orchestrator) Bus() *state.Bus { return o.bus }

// Processing returns the processing-state publisher.
func (o *Orchestrator) Processing() *state.ProcessingPublisher { return o.proc }

// Session returns the active session identity.
func (o *Orchestrator) Session() session.Identity { return o.id }

// Registry returns the surface registry external surfaces attach to (TRN-1).
func (o *Orchestrator) Registry() *surfaces.Registry { return o.surfaceReg }

// AttachToken returns the session's ephemeral attach token. The raw value is
// used to build launch command strings; only its fingerprint is ever persisted.
func (o *Orchestrator) AttachToken() surfaces.AttachToken { return o.token }

// Endpoint returns the transport endpoint external surfaces attach to, or "" when
// the transport is disabled.
func (o *Orchestrator) Endpoint() string { return o.endpoint }

// CheckModel verifies the configured model is available (CHT-C4). It is called
// after Start, before prompts are accepted, so an unavailable model is reported
// clearly rather than surfacing as a per-prompt failure. ctx bounds the probe.
func (o *Orchestrator) CheckModel(ctx context.Context) error {
	o.mu.Lock()
	model := o.model
	name := o.settings.OllamaModel
	o.mu.Unlock()
	if model == nil {
		return fmt.Errorf("orchestrator not started: no model")
	}
	if err := model.Ready(ctx, name); err != nil {
		return fmt.Errorf("model %q is not available: %w", name, err)
	}
	return nil
}

// Submit runs one prompt cycle (CHT-C3): it records the user prompt, drives the
// model through the respond phase streaming agent_response deltas onto the bus,
// and transitions processing-state idle→working→completed. A model error routes
// an error event and transitions to failed. Event ordering is deterministic:
// user_prompt, then agent_response deltas in stream order, then the terminal
// processing-state. Canceling ctx interrupts the in-flight model call: any
// partial response is kept, no error is recorded, and the cycle ends completed.
func (o *Orchestrator) Submit(ctx context.Context, text string) error {
	return o.runPrompt(ctx, text, true, true)
}

// SubmitBootstrap submits the configured bootstrap prompt at startup (story:
// bootstrap prompt). It runs the normal cycle with instructions prefixed but
// does not record a user-prompt entry, so the model response is the first thing
// shown. It is a no-op when no bootstrap prompt is configured.
func (o *Orchestrator) SubmitBootstrap(ctx context.Context) error {
	o.mu.Lock()
	text := o.settings.BootstrapPrompt
	o.mu.Unlock()
	if text == "" {
		return nil
	}
	// Bootstrap skips classification so the response is the first thing shown.
	return o.runPrompt(ctx, text, false, false)
}

// runPrompt drives one prompt cycle. When recordUserPrompt is false the user
// message is still sent to the model (so instructions + prompt reach the LLM) but
// no user_prompt event is published — used for the bootstrap prompt. When
// classifyPrompt is true the prompt is classified (and a classification event
// published) before the respond phase.
func (o *Orchestrator) runPrompt(ctx context.Context, text string, recordUserPrompt, classifyPrompt bool) error {
	o.mu.Lock()
	ready := o.started && o.accepting
	classifier := o.classifier
	o.mu.Unlock()
	if !ready {
		return fmt.Errorf("orchestrator not accepting prompts")
	}

	if classifyPrompt {
		o.setProcessing(state.StateWorking, state.PhaseClassify)
	}
	if recordUserPrompt {
		o.publish("USER_PROMPT", state.ContentUserPrompt, map[string]any{"text": text})
	}

	route := ""
	if classifyPrompt && classifier != nil {
		verdict := classifier.Classify(ctx, text)
		route = string(verdict.Route)
		o.publish("CLASSIFICATION", state.ContentClassification, map[string]any{
			"route":     route,
			"rationale": verdict.Rationale,
			"text":      classificationText(verdict),
		})
		// v1: only respond_directly executes; reserved routes fall back to respond.
	}

	// Single-tool execution cycle: propose → policy/approval → execute → answer
	// with the result folded in. A reserved route, disabled tools, or a no-tool
	// proposal fall through to a normal answer.
	if route == string(classify.SingleTool) && o.toolsReady() {
		toolCtx, handled, terr := o.runToolPhase(ctx, text)
		if terr != nil {
			// Interrupted while awaiting approval: end the cycle cleanly.
			o.setProcessing(state.StateCompleted, state.PhaseNone)
			return nil
		}
		if handled {
			msgs := o.withContext(o.assembler.Assemble(text + toolCtx))
			resp, err := o.streamResponse(ctx, msgs, nil, false)
			o.recordTurn(err, recordUserPrompt, text, resp)
			return o.finishCycle(err)
		}
	}

	// Route-aware thinking: the verdict decides whether this turn reasons before
	// answering, with a wall-clock budget that falls back to a direct answer.
	doThink := o.thinkForRoute(route)
	messages := o.withContext(o.assembler.AssembleWithThinking(text, o.thinkingPrompt(doThink), route))
	fallback := o.withContext(o.assembler.Assemble(text))
	resp, err := o.streamResponse(ctx, messages, fallback, doThink)
	o.recordTurn(err, recordUserPrompt, text, resp)
	return o.finishCycle(err)
}

// recordTurn appends the completed turn to the in-memory conversation history when
// the cycle ended cleanly (success or user interrupt), so the next turn carries
// the prior user prompt and agent response as enabled context. The bootstrap turn
// (recordTurn=false, like its user-prompt event) is excluded: it engages the
// session but is irrelevant to the user's intent. Hard failures are not recorded.
func (o *Orchestrator) recordTurn(err error, record bool, userText, response string) {
	if !record {
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if userText != "" {
		o.history = append(o.history, prompting.Message{Role: "user", Content: userText})
	}
	if response != "" {
		o.history = append(o.history, prompting.Message{Role: "assistant", Content: response})
	}
}

// historyMessages returns a copy of the enabled prior-turn conversation history.
func (o *Orchestrator) historyMessages() []prompting.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]prompting.Message, len(o.history))
	copy(out, o.history)
	return out
}

// seedWorkingMemory writes the bootstrap facts (userid, cwd) into the session's
// working_memory.json when absent, leaving any existing user-managed facts
// (including edits and disabled state) intact.
func (o *Orchestrator) seedWorkingMemory() error {
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return err
	}
	if wm.SeedIfAbsent(session.BootstrapFacts()...) {
		return o.store.SaveWorkingMemory(o.id.ID, wm)
	}
	return nil
}

// withContext folds working memory and enabled prior-turn history into an
// assembled [system?, user] message list, in layer order: instructions (Layer 0)
// → working memory (band 0) → enabled conversation history → the current user
// turn. Both are re-read fresh each turn so edits/new turns take effect on the
// next post.
func (o *Orchestrator) withContext(msgs []prompting.Message) []prompting.Message {
	at := 0
	for at < len(msgs) && msgs[at].Role == "system" {
		at++
	}
	out := make([]prompting.Message, 0, len(msgs)+len(o.history)+1)
	out = append(out, msgs[:at]...)
	if wmMsg, ok := o.workingMemoryMessage(); ok {
		out = append(out, wmMsg)
	}
	out = append(out, o.historyMessages()...)
	out = append(out, msgs[at:]...)
	return out
}

// workingMemoryMessage renders the session's enabled working-memory facts into a
// system message (band 0). The file is the source of truth, re-read fresh each
// turn. ok is false on a read error or an empty fact set.
func (o *Orchestrator) workingMemoryMessage() (prompting.Message, bool) {
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return prompting.Message{}, false
	}
	enabled := wm.Enabled()
	facts := make([]prompting.Fact, 0, len(enabled))
	for _, f := range enabled {
		facts = append(facts, prompting.Fact{Key: f.Key, Value: f.Value})
	}
	return prompting.WorkingMemoryMessage(facts)
}

// thinkForRoute reports whether this turn should reason before answering: the
// master switch is on and the classified route opts into thinking. An empty
// route (unclassified, e.g. bootstrap) never thinks.
func (o *Orchestrator) thinkForRoute(route string) bool {
	if !o.settings.ThinkingEnabled || route == "" {
		return false
	}
	return o.settings.ThinkingRoutes[route]
}

// thinkingPrompt returns the thinking guidance to fold into the respond system
// prompt, or "" when not thinking. Empty configured guidance uses the default.
func (o *Orchestrator) thinkingPrompt(doThink bool) string {
	if !doThink {
		return ""
	}
	if p := strings.TrimSpace(o.settings.ThinkingPrompt); p != "" {
		return p
	}
	return prompting.DefaultThinkingPrompt
}

// classificationText renders the greyed "intent → route" line for the output
// panel (see ux/06_OUTPUT_WIDGET.md).
func classificationText(v classify.Verdict) string {
	if v.Rationale != "" {
		return fmt.Sprintf("%s → %s", v.Rationale, v.Route)
	}
	return fmt.Sprintf("→ %s", v.Route)
}

// publish stamps and fans an event out over the bus.
func (o *Orchestrator) publish(eventType string, ct state.ContentType, payload any) {
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   eventType,
		ContentType: ct,
		Payload:     payload,
		Enabled:     state.DefaultEnabled(ct),
		ModelName:   o.settings.OllamaModel,
	})
}

// setProcessing updates the live processing-state feed and persists a snapshot
// onto the bus so the transition is recoverable from the event log.
func (o *Orchestrator) setProcessing(s state.RunState, ph state.Phase) {
	o.proc.Set(s, ph)
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "PROCESSING_STATE",
		ContentType: state.ContentProcessingState,
		Payload:     o.proc.Current(),
	})
}
