package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"agentx/internal/classify"
	"agentx/internal/config"
	"agentx/internal/llm/provider"
	"agentx/internal/llm/provider/validation"
	"agentx/internal/prompting"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/runtime/hooks"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/runtime/wavefront"
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
	// SessionName names the session; empty generates a default adjective-noun name.
	// A collision is disambiguated with a numeric suffix (see session.uniqueName).
	SessionName string
	// Provider selects the LLM backend: "ollama" or "llamacpp" (default "ollama").
	Provider string
	// OllamaHost and OllamaModel configure the Ollama model adapter.
	OllamaHost  string
	OllamaModel string
	// LlamacppHost and LlamacppModel configure the llama.cpp model adapter.
	LlamacppHost  string
	LlamacppModel string
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
	// ClarificationOptions is the number of interpretations offered on ambiguity.
	ClarificationOptions int
	// PromptCorpus is the fan-group prompt corpus (prompts.toml content) that drives
	// the experimental task classifier. Empty (the default, when no prompts.toml is
	// seeded) leaves the classifier off and the prompt cycle unchanged.
	PromptCorpus string
	// PlannerPrompt is the decomposition-planner system prompt (from
	// ~/.config/agentx/agentx-planner.md). Empty uses planner.DefaultPromptTemplate.
	PlannerPrompt string
	// PlannerThinkingBudget bounds the decomposition planner's own Complete-based
	// reasoning phase (ADR 0012 Phase 1), independent of ThinkingBudget below (which
	// governs the streaming respond path only). <=0 disables — the default.
	PlannerThinkingBudget time.Duration
	// WavefrontClassifyPrompt is the wavefront classify system prompt (from
	// ~/.config/agentx/agentx-wavefront-classify.md). Empty uses
	// wavefront.DefaultClassifyPromptTemplate. ADR 0012 Phase 2 — not yet consumed by
	// any runtime path (the wavefront engine itself lands in a later phase).
	WavefrontClassifyPrompt string
	// WavefrontSynthesisPrompt is the wavefront synthesis system prompt (from
	// ~/.config/agentx/agentx-wavefront-synthesis.md). Empty uses
	// wavefront.DefaultSynthesisPromptTemplate. ADR 0012 Phase 2.
	WavefrontSynthesisPrompt string
	// WavefrontSummaryPrompt is the wavefront output-summarization system prompt
	// (from ~/.config/agentx/agentx-wavefront-summary.md). Empty uses
	// wavefront.DefaultSummaryPromptTemplate. ADR 0012 Phase 2.
	WavefrontSummaryPrompt string
	// MaxWidgetLines is the output-widget body-row cap surfaced to the chat UI.
	MaxWidgetLines int
	// InputMaxLines caps how tall the input panel grows before it scrolls.
	InputMaxLines int
	// MarkdownRenderer selects assistant-markdown styling: "native" or "scanner"
	// (ADR 0007). Surfaced to the chat UI.
	MarkdownRenderer string
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
	// WavefrontEnabled routes invoke_planner plans through ADR 0012's round-free
	// decomposition engine instead of the continuous engine (default off).
	WavefrontEnabled bool
	// ToolReadOnly restricts execution to read-risk tools (the rollout default).
	ToolReadOnly bool
	// ToolCatalog is the LLM-facing tool catalog injected into the proposal prompt
	// (from agentx-shell-commands.md). Empty uses tools.DefaultCatalog.
	ToolCatalog string
	// ToolBlacklistPath and ToolApprovalsPath persist the command policy across
	// sessions (blacklist rules in, global approvals in/out). Empty disables I/O.
	ToolBlacklistPath string
	ToolApprovalsPath string
	// ContinuationVerbsAllowedPath and ContinuationVerbsDeniedPath persist the
	// verb-continuation allow/deny lists (from ~/.config/agentx/agentx-continuation-
	// verbs-allowed.md / -denied.md) — an "always" decision appends to whichever list
	// applies. Empty disables persistence (an "always" decision behaves like "once").
	ContinuationVerbsAllowedPath string
	ContinuationVerbsDeniedPath  string
	// ToolTimeoutSeconds bounds the wall-clock budget for a single tool
	// execution. <=0 lets the executor use its own default.
	ToolTimeoutSeconds int
	// ToolOutputMaxBytes caps captured tool output before truncation (full output
	// still persists to the artifact). <=0 uses the executor default.
	ToolOutputMaxBytes int
	// ToolOutputOverridesPath persists per-tool oversized-output recovery decisions
	// (TOOL-6): an "always" choice is remembered here. Empty disables persistence
	// (an "always" decision behaves like "once").
	ToolOutputOverridesPath string
	// ToolOutputAbsoluteMaxBytes is the hard ceiling the recovery gate's "capture
	// more" choice is clamped to, regardless of interactive choice or remembered
	// preference. <=0 uses the config default (2 MiB).
	ToolOutputAbsoluteMaxBytes int
	// TransportEnabled serves the HTTP/SSE transport alongside the in-process chat
	// so external surfaces can attach. When false, the runtime stays in-process.
	TransportEnabled bool
	// TransportHost is the loopback host the transport binds (e.g. 127.0.0.1).
	TransportHost string
	// TransportPortStart and TransportPortEnd bound the candidate port range the
	// transport binds the first free port from.
	TransportPortStart int
	TransportPortEnd   int
	// ConfigWatcherPath is the path the config-file watcher monitors (Phase 3a).
	// Empty means the orchestrator resolves the path from config.DefaultPaths()
	// at start time (the conventional deployment path).
	ConfigWatcherPath string
	// MaxToolIterationsPerTurn caps how many native tool-call round-trips one turn
	// may run before the loop stops and answers with whatever it has. <=0 uses the
	// built-in default (25) — unbounded native tool-calling needs a runaway guard
	// the old one-tool-per-turn cycle never required.
	MaxToolIterationsPerTurn int
	// HooksConfigPath is a TOML file registering synchronous/asynchronous loop
	// hooks (see internal/runtime/hooks). Empty (the default) registers no hooks —
	// the framework is present but unused until a future hook implementation ships
	// and is named here.
	HooksConfigPath string
}

// configPayload is an in-memory snapshot of the latest config write, stored so
// a restart can reapply it. See restartQueue on Orchestrator (Phase 1e).
type configPayload = map[string]any

// Orchestrator owns the per-process runtime: session, event bus, processing
// state, and persistence.
type Orchestrator struct {
	settings Settings

	store        *session.Store
	id           session.Identity
	bus          *state.Bus
	proc         *state.ProcessingPublisher
	token        surfaces.AttachToken
	surfaceReg   *surfaces.Registry
	server       *transporthttp.Server
	endpoint     string
	serveDone    chan error
	model        Model
	assembler    *prompting.Assembler
	classifier   *classify.Classifier
	taskPipeline *pipeline.Pipeline
	taskExec     taskExecutor
	taskDecomp   scheduler.Decomposer
	// outputSummarizer condenses an oversized captured finding (ADR 0012 §6, Phase 3);
	// nil until buildDecomposition wires it, and capturingExec degrades to plain
	// truncation when nil, same posture as a nil artifactReader.
	outputSummarizer wavefront.CondenseFunc
	// wavefrontClassifier/wavefrontChat back ADR 0012's second decomposition engine
	// (Phase 8); nil until buildWavefront wires them (gated on Settings.WavefrontEnabled
	// — an unused engine costs nothing). wavefrontChat serves both the classifier's
	// schema-constrained calls and the scheduler's schema-free synthesis calls — the
	// same closure, since Format is just a per-call parameter, not two closures
	// differing only in whether they set one.
	wavefrontClassifier wavefront.Classifier
	wavefrontChat       wavefront.Chat
	recDone             chan error
	recSub              *state.Subscription
	gate                decisionGate

	// configWatcher monitors agentx.toml for external edits (Phase 3a, PD-CONFIG-AF-008)
	// and fans config_changed events to attached surfaces via the Bus.
	configWatcher     *config.Watcher
	configWatcherStop context.CancelFunc

	// restartQueue holds pending config changes that require a restart (e.g.
	// provider switch, host change). Applied when the orchestrator restarts via
	// Restart() (Phase 1e, PD-CONFIG-AF-009).
	restartQueue configPayload
	// liveReloadEnabled reports whether tunable settings should be applied
	// immediately to the running session without restart (Phase 1e).
	liveReloadEnabled bool
	registry          *tools.Registry
	policy            *tools.Policy
	runner            ToolRunner
	outputOverrides   *tools.OutputOverrides
	planTrees         *planTreeRegistry
	// hooks is the loop's sync/async extension registry (internal/runtime/hooks).
	// Built at Start from Settings.HooksConfigPath; empty (no-op) when unset.
	hooks *hooks.Registry
	// core is the extracted prompt/tool/hook loop (ADR 0013). Built at Start by
	// buildCore, once o.hooks and o.assembler exist. runPrompt delegates to it.
	core *ConversationCore
	// planSeq mints unique plan_task root ids across this orchestrator's lifetime
	// (atomic — a turn may call plan_task more than once, and calls execute
	// sequentially but the counter itself must still be safe under WithX test
	// injection races).
	planSeq uint64

	mu        sync.Mutex
	started   bool
	accepting bool
	history   []turnMsg
	ctxWindow int // cached model context length (tokens); 0 = not yet resolved
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

// WithConfigWatcherPath overrides the config file path the watcher monitors
// (Phase 3a test hook). Without it the orchestrator resolves the path from
// config.DefaultPaths().deployment at start time.
func WithConfigWatcherPath(path string) Option {
	return func(o *Orchestrator) { o.settings.ConfigWatcherPath = path }
}

// New returns an unstarted Orchestrator for the given settings.
func New(s Settings, opts ...Option) *Orchestrator {
	o := &Orchestrator{settings: s, planTrees: newPlanTreeRegistry()}
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
	var createOpts []session.Option
	if name := strings.TrimSpace(o.settings.SessionName); name != "" {
		createOpts = append(createOpts, session.WithNamer(func() string { return name }))
	}
	id, err := o.store.Create(createOpts...)
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
		switch strings.ToLower(o.settings.Provider) {
		case "llamacpp":
			o.model = newLlamacppModel(o.settings.LlamacppHost, o.settings.LlamacppModel)
		default:
			o.model = newOllamaModel(o.settings.OllamaHost)
		}
	}
	instructions := o.settings.Instructions
	if instructions == "" {
		instructions = prompting.DefaultSystemPrompt
	}
	o.assembler = prompting.New(instructions)
	// classify/continuation/the task-classifier pipeline are disconnected from the
	// main loop (native tool-calling replaces their job) but deliberately left in
	// the tree, unwired — see docs/implementation and the simplified-workflow
	// design discussion. o.classifier/o.taskPipeline are no longer constructed
	// here; WithClassifier/WithTaskClassifier still exist for tests that want to
	// inject one directly.
	if o.settings.ToolsEnabled {
		if err := o.buildTools(); err != nil {
			return err
		}
	}
	// The task executor drains resolved task records into verified effects; it
	// needs the tool collaborators, so it is built after buildTools. It now backs
	// plan_task only (not gated on the task-classifier pipeline — see
	// buildTaskExecutor's comment).
	o.buildTaskExecutor()
	// Decomposition (the plan_task tool) needs the executor; build it last.
	o.buildDecomposition()
	o.buildWavefront()
	hookConfigs, err := hooks.LoadConfig(o.settings.HooksConfigPath)
	if err != nil {
		return fmt.Errorf("load hooks config: %w", err)
	}
	o.hooks, err = hooks.Build(hookConfigs)
	if err != nil {
		return fmt.Errorf("build hooks: %w", err)
	}
	o.buildCore()

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

	// Phase 3a: start the config-file watcher so external edits to agentx.toml
	// are fanned out as config_changed events to attached surfaces.
	if err := o.startConfigWatcher(); err != nil {
		// Non-fatal: a failed watcher start does not block the orchestrator.
		// The bus and transport are still live; only external-change detection
		// is disabled.
		_ = err
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
	if err := o.store.WriteTransport(o.id.ID, session.TransportInfo{SessionID: o.id.ID, SessionName: o.id.Name, Endpoint: o.endpoint}); err != nil {
		_ = ln.Close()
		return fmt.Errorf("publish transport endpoint: %w", err)
	}
	// Publish the raw attach token (0600) so same-machine peer launches resolve it
	// from disk without the user copying it (SS-5).
	if err := o.store.WriteAttachToken(o.id.ID, o.token.Raw()); err != nil {
		_ = ln.Close()
		return fmt.Errorf("publish attach token: %w", err)
	}
	o.server = transporthttp.NewServer(o)
	o.serveDone = make(chan error, 1)
	go func() { o.serveDone <- o.server.Serve(ln) }()
	return nil
}

// startConfigWatcher begins the config-file watcher (Phase 3a, PD-CONFIG-AF-008).
// It watches agentx.toml for external modifications and publishes a config_changed
// event to the bus whenever one is detected. The watcher runs in its own goroutine
// and is cleaned up in Shutdown.
func (o *Orchestrator) startConfigWatcher() error {
	configPath := o.settings.ConfigWatcherPath
	if configPath == "" {
		paths, err := config.DefaultPaths()
		if err != nil {
			return err
		}
		configPath = paths.Deployment
	}

	w, err := config.NewWatcher(configPath)
	if err != nil {
		return err
	}
	o.configWatcher = w

	// Subscribe to the watcher and fan events into the bus.
	sub := w.Subscribe()
	go func() {
		for range sub.C {
			// Publish a config_changed event. The bus stamps it with an ordinal
			// and fans it to every subscriber (including SSE-attached surfaces).
			ordinal := o.bus.Publish(state.Event{
				Epoch:       time.Now().UnixMilli(),
				SessionID:   o.id.ID,
				EventType:   "CONFIG_CHANGED",
				ContentType: state.ContentConfigChanged,
				Payload:     map[string]any{"path": configPath},
			})
			_ = ordinal
		}
	}()
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
	// mark attached surfaces stopped. The published attach token is no longer valid
	// once the server stops, so remove it from disk (SS-5).
	if server != nil {
		_ = server.Shutdown(ctx)
		o.surfaceReg.StopAll()
		_ = o.store.RemoveAttachToken(o.id.ID)
	}

	// Phase 3a: stop the config-file watcher before draining, so no new
	// config_changed events are published after the bus closes.
	if w := o.configWatcher; w != nil {
		_ = w.Close()
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

// History returns the session's persisted event log (the durable source of truth,
// including each event's enabled state and ordinal) for seeding an attaching
// surface. Read from disk so it survives independent of in-memory fan-out.
func (o *Orchestrator) History() ([]state.Event, error) {
	return o.store.Recorder(o.id.ID).Load()
}

// CheckModel verifies the configured model is available (CHT-C4). It is called
// after Start, before prompts are accepted, so an unavailable model is reported
// clearly rather than surfacing as a per-prompt failure. ctx bounds the probe.
func (o *Orchestrator) CheckModel(ctx context.Context) error {
	o.mu.Lock()
	model := o.model
	name := o.modelName()
	o.mu.Unlock()
	if model == nil {
		return fmt.Errorf("orchestrator not started: no model")
	}
	if err := model.Ready(ctx, name); err != nil {
		return fmt.Errorf("model %q is not available: %w", name, err)
	}
	return nil
}

// modelName returns the configured model name for the active provider.
func (o *Orchestrator) modelName() string {
	switch strings.ToLower(o.settings.Provider) {
	case "llamacpp":
		return o.settings.LlamacppModel
	default:
		return o.settings.OllamaModel
	}
}

// Submit runs one prompt cycle (CHT-C3): it records the user prompt, drives the
// model through the respond phase streaming agent_response deltas onto the bus,
// and transitions processing-state idle→working→completed. A model error routes
// an error event and transitions to failed. Event ordering is deterministic:
// user_prompt, then agent_response deltas in stream order, then the terminal
// processing-state. Canceling ctx interrupts the in-flight model call: any
// partial response is kept, no error is recorded, and the cycle ends completed.
func (o *Orchestrator) Submit(ctx context.Context, text string) error {
	return o.runPrompt(ctx, text, true, false)
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
	// Bootstrap's events are marked ephemeral so the context viewer omits them.
	return o.runPrompt(ctx, text, false, true)
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

// WorkingMemory returns the session's working-memory facts for a surface to read.
func (o *Orchestrator) WorkingMemory() ([]session.Fact, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return nil, err
	}
	return wm.Facts, nil
}

// SetFact upserts a working-memory fact: a new key is added (user-owned, enabled);
// an existing key has its value updated. The change persists, so it takes effect on
// the next prompt's assembled context.
func (o *Orchestrator) SetFact(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("fact key is required")
	}
	return o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		wm.Set(key, value)
		return nil
	})
}

// DeleteFact removes a working-memory fact by key. An unknown key is not an error.
func (o *Orchestrator) DeleteFact(key string) error {
	return o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		wm.Delete(key)
		return nil
	})
}

// SetFactEnabled enables or disables a working-memory fact, controlling whether it
// folds into the assembled context. An unknown key is a validation error.
func (o *Orchestrator) SetFactEnabled(key string, enabled bool) error {
	return o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		if !wm.SetEnabled(key, enabled) {
			return fmt.Errorf("unknown fact %q", key)
		}
		return nil
	})
}

// SetFactLive toggles whether a pinned fact re-runs its source tool before every
// turn's context assembly (live) or stays a frozen snapshot (static) — the
// working-memory surface's play/pause affordance (PD-WM-AF-008). It is an error
// on a fact with no Source (only a pin-owned fact has a live/static state).
// Turning live on immediately refreshes the fact once, so the toggle visibly does
// something rather than waiting for the next turn.
func (o *Orchestrator) SetFactLive(key string, live bool) error {
	if live {
		facts, err := o.WorkingMemory()
		if err != nil {
			return err
		}
		var src *session.ToolSource
		found := false
		for _, f := range facts {
			if f.Key == key {
				src, found = f.Source, true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown fact %q", key)
		}
		if src == nil {
			return fmt.Errorf("fact %q is not pinned to a tool source", key)
		}
		if !o.liveEligible(src) {
			return fmt.Errorf("cannot set %q live: %q is not currently permitted without approval", key, src.Tool)
		}
	}
	err := o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		for i := range wm.Facts {
			if wm.Facts[i].Key != key {
				continue
			}
			if wm.Facts[i].Source == nil {
				return fmt.Errorf("fact %q is not pinned to a tool source", key)
			}
			wm.Facts[i].Live = live
			return nil
		}
		return fmt.Errorf("unknown fact %q", key)
	})
	if err != nil {
		return err
	}
	if live {
		o.refreshLiveFacts(context.Background())
	}
	return nil
}

// liveEligible reports whether src's tool currently evaluates to policy Allow —
// the gate both PinToolEvent(live=true) and SetFactLive(true) apply, so a live
// pin never silently re-runs something that would otherwise need approval.
func (o *Orchestrator) liveEligible(src *session.ToolSource) bool {
	o.mu.Lock()
	registry, policy := o.registry, o.policy
	o.mu.Unlock()
	if registry == nil || policy == nil {
		return false
	}
	d, ok := registry.Lookup(src.Tool)
	if !ok {
		return false
	}
	return policy.Evaluate(d, src.Args).Decision == tools.Allow
}

// PinToolEvent copies a tool_result conversation element (by ordinal) into
// working memory as a durable, pin-owned fact, and disables the source event's
// context-surface participation so the same tool output is never represented
// twice — once as a raw context element, once as a WM fact
// (docs/ux/03_PANEL_DETAILS.md PD-CTX-AF-012 / PD-WM). live requires the source
// tool to currently evaluate to policy Allow: a live pin must never silently
// re-run something that would otherwise need approval, and it must never block a
// turn on an approval prompt. It returns the new fact's key.
func (o *Orchestrator) PinToolEvent(ordinal uint64, live bool) (string, error) {
	hist, err := o.History()
	if err != nil {
		return "", err
	}
	var ev state.Event
	found := false
	for _, e := range hist {
		if e.Ordinal == ordinal {
			ev, found = e, true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("ordinal %d not found", ordinal)
	}
	if ev.ContentType != state.ContentToolResult {
		return "", fmt.Errorf("ordinal %d is not a tool_result element", ordinal)
	}

	p, _ := ev.Payload.(map[string]any)
	text, _ := p["text"].(string)
	var src *session.ToolSource
	if ev.ToolName != "" {
		src = &session.ToolSource{Tool: ev.ToolName, Args: stringMapFrom(p["args"])}
	}

	if live {
		if src == nil {
			return "", fmt.Errorf("cannot pin live: ordinal %d has no tool source", ordinal)
		}
		if !o.liveEligible(src) {
			return "", fmt.Errorf("cannot pin live: %q is not currently permitted without approval", src.Tool)
		}
	}

	fact := session.Fact{
		Key: pinFactKey(ev.ToolName, ordinal), Value: text, Owner: session.OwnerPin, Enabled: true,
		Source: src, Live: live, SourceOrdinal: ordinal, PinnedAt: time.Now(),
	}
	if err := o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		wm.Facts = append(wm.Facts, fact)
		return nil
	}); err != nil {
		return "", err
	}
	if err := o.SetEventEnabled(ordinal, false); err != nil {
		return fact.Key, err // fact created but disabling the source failed — key is still usable
	}
	return fact.Key, nil
}

// pinFactKey names the working-memory fact a Pin creates: the tool id plus the
// source ordinal, so the same tool pinned twice from different turns gets
// distinct keys and the name stays traceable back to its source event. No
// "pin_" prefix: Owner already records that this fact is pin-owned, so
// repeating it in the key would stutter (every WM fact this shape produces
// is, by construction, a pin).
func pinFactKey(tool string, ordinal uint64) string {
	return fmt.Sprintf("%s_%d", tool, ordinal)
}

// PinPlanNode copies a plan node's own resolved Value into working memory as a
// durable, pin-owned fact with no tool Source (ADR 0012 amendment, surface-
// visibility follow-up) — the counterpart to PinToolEvent for a node with no
// backing tool call at all (a Step, e.g. a wavefront Know: its Value comes from
// the classify response's own Knows or the fallback synthesis call, never the
// executor). Reads the node's current goal/value straight from the durable plan
// tree (the same authoritative source PinToolEvent reads via o.History(), not
// whatever text a client last rendered), so it is refused, not silently pinned
// empty, once the node has nothing resolved yet. Unlike PinToolEvent, there is no
// live option: a Source-less fact has nothing to re-run, so it can only ever be
// static (mirrors PD-WM-AF-009's existing policy-Allow-only-live refusal, applied
// here at pin time instead of toggle time since there is no tool at all to gate
// on). It returns the new fact's key.
func (o *Orchestrator) PinPlanNode(root, nodeID string) (string, error) {
	goal, value, _, ok := o.planTrees.node(root, nodeID)
	if !ok {
		return "", fmt.Errorf("plan %q node %q not found", root, nodeID)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("node %q has no resolved value to pin", nodeID)
	}
	fact := session.Fact{
		Key: pinNodeFactKey(goal, nodeID), Value: value, Owner: session.OwnerPin,
		Enabled: true, PinnedAt: time.Now(),
	}
	if err := o.mutateWorkingMemory(func(wm *session.WorkingMemory) error {
		wm.Facts = append(wm.Facts, fact)
		return nil
	}); err != nil {
		return "", err
	}
	return fact.Key, nil
}

// pinNodeFactKey names the working-memory fact a plan-node Pin creates: the
// node's own goal text (human-readable, unlike pinFactKey's tool+ordinal shape,
// since a Step's goal already reads as the fact's name — "the project's dominant
// language" is a better WM key than an opaque node id) plus the node id for
// uniqueness, mirroring pinFactKey's "readable part + unique part" shape.
func pinNodeFactKey(goal, nodeID string) string {
	s := strings.ToLower(strings.Join(strings.Fields(goal), "-"))
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		return nodeID
	}
	return s + "_" + nodeID
}

// stringMapFrom coerces a JSON-decoded args value (map[string]any, string
// leaves — state.Event.Payload always arrives this shape once round-tripped
// through the durable log) into a map[string]string. Any non-string value or a
// non-map input is dropped/nil respectively.
func stringMapFrom(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

// refreshLiveFacts re-runs every enabled, live, pin-owned fact's source tool and
// updates its value before context assembly (PD-WM "pin live"), called once at
// the top of runPrompt. A refresh failure (unknown/no-longer-permitted tool,
// execution error) keeps the fact's stale value rather than failing the turn —
// working memory degrades gracefully, the same posture as the
// context-visualizer's unknown-window handling. All refreshed facts are batched
// into one load/save pass.
func (o *Orchestrator) refreshLiveFacts(ctx context.Context) {
	o.mu.Lock()
	registry, policy, runner := o.registry, o.policy, o.runner
	o.mu.Unlock()
	if registry == nil || runner == nil {
		return // tools not built (ToolsEnabled off) — nothing to refresh
	}
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return
	}
	changed := false
	for i := range wm.Facts {
		f := &wm.Facts[i]
		if !f.Enabled || !f.Live || f.Source == nil {
			continue
		}
		d, ok := registry.Lookup(f.Source.Tool)
		if !ok {
			continue
		}
		if policy != nil && policy.Evaluate(d, f.Source.Args).Decision != tools.Allow {
			continue // no longer permitted (blacklist/approval changed since pinning) — leave stale
		}
		res, err := runner.Run(ctx, d, f.Source.Args)
		if err != nil {
			continue
		}
		f.Value = toolResultText(res)
		f.RefreshedAt = time.Now()
		changed = true
	}
	if changed {
		_ = o.store.SaveWorkingMemory(o.id.ID, wm)
	}
}

// mutateWorkingMemory loads, mutates, and persists working memory under the lock.
func (o *Orchestrator) mutateWorkingMemory(fn func(*session.WorkingMemory) error) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return err
	}
	if err := fn(wm); err != nil {
		return err
	}
	return o.store.SaveWorkingMemory(o.id.ID, wm)
}

// ContextBreakdown reports the composition of the session's standing context
// window by content class (working memory, instructions, user, assistant, tools)
// for the read-only context-visualizer surface. Sizes are the exact bytes
// withContext would assemble; the window is the model's context length. "tools" is
// only non-zero once the context surface has pinned a tool_call/tool_result
// (PD-CTX-AF-011) — until then it renders as zero, like the other content classes
// not yet fed into context (attachments, thinking).
func (o *Orchestrator) ContextBreakdown() (session.ContextReport, error) {
	comps := make([]session.ContextComponent, 0, 4)

	// Working memory (band 0), the enabled facts rendered as a system message.
	if wm, ok := o.workingMemoryMessage(); ok {
		comps = append(comps, session.ContextComponent{Class: "working-memory", Chars: len(wm.Content)})
	}
	// Instructions (Layer 0): the assembler's standing system prompt.
	if o.assembler != nil {
		for _, m := range o.assembler.Assemble("") {
			if m.Role == "system" && m.Content != "" {
				comps = append(comps, session.ContextComponent{Class: "instructions", Chars: len(m.Content)})
			}
		}
	}
	// Enabled conversation history, summed by role into user/assistant/tools classes.
	// Read from o.history directly (not historyMessages, which folds a pinned "tool"
	// entry into a "user"-role message for the model) so a pin's bytes land in their
	// own "tools" band rather than misattributed to "user".
	o.mu.Lock()
	hist := append([]turnMsg(nil), o.history...)
	o.mu.Unlock()
	var userChars, asstChars, toolChars int
	for _, h := range hist {
		if !h.enabled {
			continue
		}
		switch h.role {
		case "user":
			userChars += len(h.content)
		case "assistant":
			asstChars += len(h.content)
		case "tool":
			toolChars += len(h.content)
		}
	}
	if userChars > 0 {
		comps = append(comps, session.ContextComponent{Class: "user", Chars: userChars})
	}
	if asstChars > 0 {
		comps = append(comps, session.ContextComponent{Class: "assistant", Chars: asstChars})
	}
	if toolChars > 0 {
		comps = append(comps, session.ContextComponent{Class: "tools", Chars: toolChars})
	}

	o.mu.Lock()
	model := o.modelName()
	o.mu.Unlock()
	return session.ContextReport{
		Model:        model,
		WindowTokens: o.contextWindow(),
		Components:   comps,
	}, nil
}

// SetEventEnabled toggles whether a conversation element participates in the
// agent's upcoming context. It flips the in-memory history entry (so the next
// prompt reflects it) and rewrites the element's persisted event file (so a
// re-attaching surface seeds the correct state). It is an error to toggle an
// ordinal that is not a user/agent conversation element.
func (o *Orchestrator) SetEventEnabled(ordinal uint64, enabled bool) error {
	o.mu.Lock()
	found := false
	for i := range o.history {
		if o.history[i].ordinal == ordinal {
			o.history[i].enabled = enabled
			found = true
			break
		}
	}
	o.mu.Unlock()
	if !found {
		return fmt.Errorf("ordinal %d is not a toggleable conversation element", ordinal)
	}
	if _, err := o.store.Recorder(o.id.ID).SetEnabled(ordinal, enabled); err != nil {
		return err
	}
	return nil
}

// contextWindow returns the active model's context length in tokens, cached after
// the first lookup (a fixed model property). A lookup failure returns 0 (unknown),
// which the visualizer renders without a budget percentage.
func (o *Orchestrator) contextWindow() int {
	o.mu.Lock()
	if o.ctxWindow > 0 {
		w := o.ctxWindow
		o.mu.Unlock()
		return w
	}
	model, name := o.model, o.modelName()
	o.mu.Unlock()
	if model == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := model.ContextLength(ctx, name)
	if err != nil || n <= 0 {
		return 0
	}
	o.mu.Lock()
	o.ctxWindow = n
	o.mu.Unlock()
	return n
}

// artifactStore returns the session's artifact store, tolerating a lookup failure by
// returning nil — an auxiliary capability (widening a plan step's findings beyond its UI
// preview) degrading gracefully is preferable to a hard failure over it.
func (o *Orchestrator) artifactStore() artifactReader {
	art, err := o.store.Artifacts(o.id.ID)
	if err != nil {
		return nil
	}
	return art
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
	o.publishEv(eventType, ct, payload, false)
}

// publishEv publishes an event, marking it ephemeral when it engages the session but
// is not part of the user's conversation (the bootstrap exchange). Ephemeral events
// still reach the chat surface; read-only observers like the context viewer omit them.
// It returns the stamped ordinal, the element's durable identity (used to record the
// turn and to target enable/disable toggles).
func (o *Orchestrator) publishEv(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64 {
	return o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   eventType,
		ContentType: ct,
		Payload:     payload,
		Enabled:     state.DefaultEnabled(ct),
		ModelName:   o.modelName(),
		Ephemeral:   ephemeral,
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

// Config returns the current effective configuration for transport.
func (o *Orchestrator) Config() map[string]any {
	// Phase 1e: return the full effective configuration so the config surface
	// can render all editable keys. Live-reloadable and restart-required keys
	// are both included; the surface uses ConfigSchema to determine editability.
	return map[string]any{
		"provider":                             o.settings.Provider,
		"ollama_host":                          o.settings.OllamaHost,
		"ollama_model":                         o.settings.OllamaModel,
		"llamacpp_host":                        o.settings.LlamacppHost,
		"llamacpp_model":                       o.settings.LlamacppModel,
		"classification.retries":               o.settings.ClassificationRetries,
		"classification.clarification_options": o.settings.ClarificationOptions,
		"output.max_widget_lines":              o.settings.MaxWidgetLines,
		"output.input_max_lines":               o.settings.InputMaxLines,
		"output.markdown_renderer":             o.settings.MarkdownRenderer,
		"agentx.theme.active_border_color":     o.settings.ActiveBorderColor,
		"agentx.theme.inactive_border_color":   o.settings.InactiveBorderColor,
		"thinking.enabled":                     o.settings.ThinkingEnabled,
		"thinking.time_budget_seconds":         int(o.settings.ThinkingBudget.Seconds()),
		"tools.enabled":                        o.settings.ToolsEnabled,
		"tools.read_only":                      o.settings.ToolReadOnly,
		"tools.timeout_seconds":                o.settings.ToolTimeoutSeconds,
		"tools.output_max_bytes":               o.settings.ToolOutputMaxBytes,
		"tools.absolute_max_bytes":             o.settings.ToolOutputAbsoluteMaxBytes,
		"transport.enabled":                    o.settings.TransportEnabled,
		"transport.host":                       o.settings.TransportHost,
		"transport.port_start":                 o.settings.TransportPortStart,
		"transport.port_end":                   o.settings.TransportPortEnd,
		"wavefront.enabled":                    o.settings.WavefrontEnabled,
	}
}

// ConfigSchema returns the configuration schema for transport.
func (o *Orchestrator) ConfigSchema() map[string]provider.SchemaField {
	// Phase 1e: the schema is the authoritative source of truth for the config
	// surface. It lists every editable key, its type, validation rules, and
	// whether a change requires restart. The surface uses this to render the
	// correct editor (text, dropdown, toggle, color picker) per key.
	return map[string]provider.SchemaField{
		// --- [agentx]: provider identity ---
		"provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			Required:        true,
			ReadOnly:        false,
			Description:     "The LLM backend to use: 'ollama' or 'llamacpp'.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
		// --- [agentx.ollama] ---
		"ollama_host": {
			Name:            "Ollama Host",
			Type:            "host",
			Default:         "localhost:11434",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama host address (host:port).",
			RestartRequired: true,
		},
		"ollama_model": {
			Name:            "Ollama Model",
			Type:            "model",
			Default:         "",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama model name (e.g., 'llama3.1').",
			RestartRequired: true,
		},
		// --- [agentx.llamacpp] ---
		"llamacpp_host": {
			Name:            "llama.cpp Host",
			Type:            "host",
			Default:         "localhost:8080",
			Required:        true,
			ReadOnly:        false,
			Description:     "The llama.cpp server host address (host:port).",
			RestartRequired: true,
		},
		"llamacpp_model": {
			Name:            "llama.cpp Model",
			Type:            "model",
			Default:         "",
			Required:        true,
			ReadOnly:        false,
			Description:     "The llama.cpp model name (e.g., 'llama3.1').",
			RestartRequired: true,
		},
		// --- [agentx.classification] (live-reload) ---
		"classification.retries": {
			Name:            "Classification Retries",
			Type:            "int",
			Default:         "2",
			Required:        false,
			ReadOnly:        false,
			Description:     "How many retry attempts the classify cycle makes on a non-JSON verdict.",
			RestartRequired: false,
		},
		"classification.clarification_options": {
			Name:            "Clarification Options",
			Type:            "int",
			Default:         "3",
			Required:        false,
			ReadOnly:        false,
			Description:     "Number of interpretation options offered to the user on ambiguous input.",
			RestartRequired: false,
		},
		// --- [agentx.output] (live-reload) ---
		"output.max_widget_lines": {
			Name:            "Max Widget Lines",
			Type:            "int",
			Default:         "20",
			Required:        false,
			ReadOnly:        false,
			Description:     "Maximum body rows before an output widget scrolls.",
			RestartRequired: false,
		},
		"output.input_max_lines": {
			Name:            "Input Max Lines",
			Type:            "int",
			Default:         "8",
			Required:        false,
			ReadOnly:        false,
			Description:     "Maximum rows the input panel grows to before scrolling.",
			RestartRequired: false,
		},
		"output.markdown_renderer": {
			Name:            "Markdown Renderer",
			Type:            "enum",
			Default:         "native",
			Required:        false,
			ReadOnly:        false,
			Description:     "Assistant-markdown rendering mode: 'native' (full) or 'scanner' (lightweight).",
			EnumValues:      []string{"native", "scanner"},
			RestartRequired: false,
		},
		// --- [agentx.theme] (live-reload) ---
		"agentx.theme.active_border_color": {
			Name:            "Active Border Color",
			Type:            "color",
			Default:         "cyan",
			Required:        false,
			ReadOnly:        false,
			Description:     "SGR foreground for the focused panel border. Accepts a named color, ANSI 256 index (0-255), or hex (#RRGGBB).",
			RestartRequired: false,
		},
		"agentx.theme.inactive_border_color": {
			Name:            "Inactive Border Color",
			Type:            "color",
			Default:         "dark gray",
			Required:        false,
			ReadOnly:        false,
			Description:     "SGR foreground for unfocused panel borders. Accepts a named color, ANSI 256 index (0-255), or hex (#RRGGBB).",
			RestartRequired: false,
		},
		// --- [agentx.thinking] (live-reload) ---
		"thinking.enabled": {
			Name:            "Thinking Enabled",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Master switch for model reasoning during the respond phase.",
			RestartRequired: false,
		},
		"thinking.time_budget_seconds": {
			Name:            "Thinking Time Budget",
			Type:            "int",
			Default:         "180",
			Required:        false,
			ReadOnly:        false,
			Description:     "Wall-clock cap on the thinking phase in seconds before falling back to a direct answer.",
			RestartRequired: false,
		},
		"thinking.routes.respond_directly": {
			Name:            "Thinking on Respond Directly",
			Type:            "bool",
			Default:         "false",
			Required:        false,
			ReadOnly:        false,
			Description:     "Enable thinking for the respond_directly classification route.",
			RestartRequired: false,
		},
		"thinking.routes.single_tool": {
			Name:            "Thinking on Single Tool",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Enable thinking for the single_tool classification route.",
			RestartRequired: false,
		},
		"thinking.routes.invoke_planner": {
			Name:            "Thinking on Invoke Planner",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Enable thinking for the invoke_planner classification route.",
			RestartRequired: false,
		},
		// --- [agentx.tools] (live-reload) ---
		"tools.enabled": {
			Name:            "Tools Enabled",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Turn on the single_tool execution cycle.",
			RestartRequired: false,
		},
		"tools.read_only": {
			Name:            "Tools Read-Only",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Restrict execution to read-risk tools only.",
			RestartRequired: false,
		},
		"tools.timeout_seconds": {
			Name:            "Tool Timeout",
			Type:            "int",
			Default:         "30",
			Required:        false,
			ReadOnly:        false,
			Description:     "Tool execution timeout in seconds.",
			RestartRequired: false,
		},
		"tools.output_max_bytes": {
			Name:            "Tool Output Max Bytes",
			Type:            "int",
			Default:         "65536",
			Required:        false,
			ReadOnly:        false,
			Description:     "Captured tool output cap before truncation (bytes).",
			RestartRequired: false,
		},
		"tools.absolute_max_bytes": {
			Name:            "Tool Absolute Max Bytes",
			Type:            "int",
			Default:         "2097152",
			Required:        false,
			ReadOnly:        false,
			Description:     "Hard ceiling on captured tool output (bytes); the oversized-output recovery gate never asks for more than this.",
			RestartRequired: false,
		},
		// --- [agentx.transport] (restart-required) ---
		"transport.enabled": {
			Name:            "Transport Enabled",
			Type:            "bool",
			Default:         "true",
			Required:        false,
			ReadOnly:        false,
			Description:     "Serve the HTTP/SSE transport alongside the in-process chat.",
			RestartRequired: true,
		},
		"transport.host": {
			Name:            "Transport Host",
			Type:            "host",
			Default:         "127.0.0.1",
			Required:        false,
			ReadOnly:        false,
			Description:     "Loopback host the transport binds (v1: loopback only).",
			RestartRequired: true,
		},
		"transport.port_start": {
			Name:            "Transport Port Start",
			Type:            "int",
			Default:         "8420",
			Required:        false,
			ReadOnly:        false,
			Description:     "Start of the inclusive candidate port range the transport binds from.",
			RestartRequired: true,
		},
		"transport.port_end": {
			Name:            "Transport Port End",
			Type:            "int",
			Default:         "8460",
			Required:        false,
			ReadOnly:        false,
			Description:     "End of the inclusive candidate port range (>= port_start).",
			RestartRequired: true,
		},
		// --- [agentx.wavefront] (live-reload) ---
		"wavefront.enabled": {
			Name:            "Wavefront Enabled",
			Type:            "bool",
			Default:         "false",
			Required:        false,
			ReadOnly:        false,
			Description:     "Route invoke_planner plans through ADR 0012's round-free decomposition engine.",
			RestartRequired: false,
		},
	}
}

// ListModels returns the list of models hosted on the active provider.
func (o *Orchestrator) ListModels() ([]string, error) {
	switch o.settings.Provider {
	case "ollama":
		// Ollama models are returned as a list of strings.
		// For now, return an empty list since we don't have a direct Ollama client here.
		// This will be filled in by Phase 1d when we wire up the actual provider.
		return []string{}, nil
	case "llamacpp":
		// llama.cpp models are returned as a list of strings.
		// For now, return an empty list since we don't have a direct llama.cpp client here.
		// This will be filled in by Phase 1d when we wire up the actual provider.
		return []string{}, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", o.settings.Provider)
	}
}

// TestHost tests a host endpoint and returns whether it's reachable.
func (o *Orchestrator) TestHost(provider, host string) error {
	switch strings.ToLower(provider) {
	case "ollama":
		// The ollama client is wired through the model layer in Phase 1d; for
		// Phase 1c we probe the /api/tags endpoint directly so the surface gets
		// a real reachability check without depending on a fully-built adapter.
		url := host
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/tags", nil)
		if err != nil {
			return fmt.Errorf("build ollama tags request: %w", err)
		}
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("ollama unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ollama tags returned status %d", resp.StatusCode)
		}
		return nil

	case "llamacpp":
		url := host
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/models", nil)
		if err != nil {
			return fmt.Errorf("build llamacpp models request: %w", err)
		}
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("llama.cpp unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("llama.cpp models returned status %d", resp.StatusCode)
		}
		return nil

	default:
		return fmt.Errorf("unknown provider: %q", provider)
	}
}

// SetConfig accepts a full config payload from the transport layer, validates
// each field with type-appropriate rules, normalizes deprecated keys, applies
// live-reloadable keys immediately, writes atomically to disk under the
// config-write semaphore, and queues restart-required keys. The returned
// ConfigWriteResult reports status, live-applied keys, restart-required keys,
// validation errors, and normalized keys.
//
// This is the orchestrator-side implementation of the POST /config transport
// endpoint (Phase 1d).
func (o *Orchestrator) SetConfig(payload map[string]any) (*transporthttp.ConfigWriteResult, error) {
	// 1. Resolve config paths for the write.
	cp, err := config.DefaultCachePaths()
	if err != nil {
		return &transporthttp.ConfigWriteResult{
			Status: "error",
			Errors: []string{"resolve cache paths: " + err.Error()},
		}, nil
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return &transporthttp.ConfigWriteResult{
			Status: "error",
			Errors: []string{"resolve config paths: " + err.Error()},
		}, nil
	}

	// 2. Decode the payload into a typed Config for validation.
	var cfg config.Config
	if err := decodeConfigPayload(payload, &cfg); err != nil {
		return &transporthttp.ConfigWriteResult{
			Status: "error",
			Errors: []string{"invalid config payload: " + err.Error()},
		}, nil
	}

	// 3. Normalize deprecated keys (chat_backend → provider).
	normalized := cfg.Normalize()
	normalizedKeys := make([]transporthttp.NormalizedKey, 0, len(normalized))
	for _, nk := range normalized {
		normalizedKeys = append(normalizedKeys, transporthttp.NormalizedKey{Old: nk.Old, New: nk.New})
	}

	// 4. Validate every field in the payload with type-appropriate rules.
	var errors []string

	// Validate provider (enum).
	if v, ok := payload["provider"]; ok {
		if s, ok := v.(string); ok {
			if err := validation.ValidateEnum(s, []string{"ollama", "llamacpp"}); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	// Validate ollama_host (host).
	if v, ok := payload["ollama_host"]; ok {
		if s, ok := v.(string); ok {
			if err := validation.ValidateHost(s); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	// Validate ollama_model (model name).
	if v, ok := payload["ollama_model"]; ok {
		if s, ok := v.(string); ok {
			if err := validation.ValidateModelName(s); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	// Validate llamacpp_host (host).
	if v, ok := payload["llamacpp_host"]; ok {
		if s, ok := v.(string); ok {
			if err := validation.ValidateHost(s); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	// Validate llamacpp_model (model name).
	if v, ok := payload["llamacpp_model"]; ok {
		if s, ok := v.(string); ok {
			if err := validation.ValidateModelName(s); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	// Validate integer fields with range checks.
	intFields := map[string]struct{ min, max int }{
		"classification.retries":               {0, 100},
		"classification.clarification_options": {1, 20},
		"output.max_widget_lines":              {1, 500},
		"output.input_max_lines":               {1, 100},
		"thinking.time_budget_seconds":         {0, 3600},
		"tools.timeout_seconds":                {1, 600},
		"tools.output_max_bytes":               {1024, 10485760},
		"tools.absolute_max_bytes":             {1024, 10485760},
		"transport.port_start":                 {1024, 65535},
		"transport.port_end":                   {1024, 65535},
	}
	for key, bounds := range intFields {
		if v, ok := payload[key]; ok {
			switch val := v.(type) {
			case float64:
				s := fmt.Sprintf("%d", int(val))
				if err := validation.ValidateInt(s, bounds.min, bounds.max); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %s", key, err.Message))
				}
			case string:
				if err := validation.ValidateInt(val, bounds.min, bounds.max); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %s", key, err.Message))
				}
			}
		}
	}

	// Validate boolean fields.
	boolFields := []string{
		"thinking.enabled",
		"tools.enabled",
		"tools.read_only",
		"transport.enabled",
		"wavefront.enabled",
	}
	for _, key := range boolFields {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				if err := validation.ValidateBool(s); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %s", key, err.Message))
				}
			}
		}
	}

	// If there are validation errors, return early with them.
	if len(errors) > 0 {
		return &transporthttp.ConfigWriteResult{
			Status:         "error",
			Errors:         errors,
			NormalizedKeys: normalizedKeys,
		}, nil
	}

	// 5. Classify keys into live-applied and restart-required (Phase 1e).
	//
	// Live-reloadable keys are tunable settings that can be applied to the
	// running session without restarting the orchestrator. They include:
	//   - Classification tuning (retries, clarification_options)
	//   - Output tuning (max_widget_lines, input_max_lines, markdown_renderer)
	//   - Theme (active_border_color, inactive_border_color)
	//   - Thinking settings (enabled, time_budget_seconds, routes.*)
	//   - Tools settings (enabled, read_only, timeout_seconds, output_max_bytes,
	//     absolute_max_bytes)
	//   - Wavefront (enabled)
	//
	// Restart-required keys change the model adapter, transport binding, or
	// provider identity and require a full orchestrator restart:
	//   - provider
	//   - ollama_host, ollama_model
	//   - llamacpp_host, llamacpp_model
	//   - transport.enabled, transport.host, transport.port_start, transport.port_end
	//
	// This classification drives what applyLiveSettings hot-applies vs what
	// restartQueue holds for the next Restart() call.
	var restartRequiredKeys, liveAppliedKeys []string
	for key := range payload {
		switch key {
		// Restart-required: these change the model adapter, transport binding,
		// or provider identity and require a full orchestrator restart.
		case "provider", "ollama_host", "ollama_model",
			"llamacpp_host", "llamacpp_model",
			"transport.enabled", "transport.host",
			"transport.port_start", "transport.port_end":
			restartRequiredKeys = append(restartRequiredKeys, key)
		default:
			liveAppliedKeys = append(liveAppliedKeys, key)
		}
	}

	// 6. Apply the config atomically to disk using the transactional write
	// infrastructure from Phase 1b.
	if err := config.WriteConfig(cp, paths.Deployment, cfg); err != nil {
		return &transporthttp.ConfigWriteResult{
			Status:          "error",
			Errors:          []string{"write config to disk: " + err.Error()},
			NormalizedKeys:  normalizedKeys,
			LiveApplied:     liveAppliedKeys,
			RestartRequired: restartRequiredKeys,
		}, nil
	}

	// Queue restart-required config for the next orchestrator restart (Phase 1e).
	o.mu.Lock()
	if len(restartRequiredKeys) > 0 {
		o.restartQueue = payload
	}
	o.mu.Unlock()

	result := &transporthttp.ConfigWriteResult{
		Status:          "applied",
		LiveApplied:     liveAppliedKeys,
		RestartRequired: restartRequiredKeys,
		NormalizedKeys:  normalizedKeys,
		Write: &transporthttp.WriteMetadata{
			Path:      paths.Deployment,
			Semaphore: cp.LockFile(),
		},
	}

	// 7. Apply live-reloadable keys to the running orchestrator state (Phase 1e).
	o.mu.Lock()
	o.applyLiveSettings(payload)
	o.mu.Unlock()

	return result, nil
}

// applyLiveSettings updates the orchestrator's in-memory settings from a
// config payload (Phase 1e). Live-reloadable keys are applied immediately
// to the running session: tunable settings take effect on the next prompt
// cycle, the next tool execution, and so on. Restart-required keys are
// silently ignored here — they are stored in o.restartQueue instead.
//
// Called under o.mu.
func (o *Orchestrator) applyLiveSettings(payload map[string]any) {
	// --- Tunable keys that can be hot-applied ---

	// Classification tuning: take effect on the next classify cycle.
	if v, ok := payload["classification.retries"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ClassificationRetries = n
		}
	}
	if v, ok := payload["classification.clarification_options"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ClarificationOptions = n
		}
	}

	// Output tuning: take effect on the next render.
	if v, ok := payload["output.max_widget_lines"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.MaxWidgetLines = n
		}
	}
	if v, ok := payload["output.input_max_lines"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.InputMaxLines = n
		}
	}
	if v, ok := payload["output.markdown_renderer"]; ok {
		if s, ok := v.(string); ok {
			o.settings.MarkdownRenderer = strings.TrimSpace(s)
		}
	}

	// Theme colors: take effect on the next render.
	if v, ok := payload["agentx.theme.active_border_color"]; ok {
		if s, ok := v.(string); ok {
			o.settings.ActiveBorderColor = s
		}
	}
	if v, ok := payload["agentx.theme.inactive_border_color"]; ok {
		if s, ok := v.(string); ok {
			o.settings.InactiveBorderColor = s
		}
	}

	// Thinking settings: take effect on the next prompt's respond phase
	// (thinkForRoute and thinkingPrompt read from o.settings each turn).
	if v, ok := payload["thinking.enabled"]; ok {
		if b, err := boolFromAny(v); err == nil {
			o.settings.ThinkingEnabled = b
		}
	}
	if v, ok := payload["thinking.time_budget_seconds"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ThinkingBudget = time.Duration(n) * time.Second
		}
	}
	if v, ok := payload["thinking.routes.respond_directly"]; ok {
		if b, err := boolFromAny(v); err == nil {
			if o.settings.ThinkingRoutes == nil {
				o.settings.ThinkingRoutes = make(map[string]bool)
			}
			o.settings.ThinkingRoutes["respond_directly"] = b
		}
	}
	if v, ok := payload["thinking.routes.single_tool"]; ok {
		if b, err := boolFromAny(v); err == nil {
			if o.settings.ThinkingRoutes == nil {
				o.settings.ThinkingRoutes = make(map[string]bool)
			}
			o.settings.ThinkingRoutes["single_tool"] = b
		}
	}
	if v, ok := payload["thinking.routes.invoke_planner"]; ok {
		if b, err := boolFromAny(v); err == nil {
			if o.settings.ThinkingRoutes == nil {
				o.settings.ThinkingRoutes = make(map[string]bool)
			}
			o.settings.ThinkingRoutes["invoke_planner"] = b
		}
	}

	// Tools settings: take effect on the next tool execution.
	if v, ok := payload["tools.enabled"]; ok {
		if b, err := boolFromAny(v); err == nil {
			o.settings.ToolsEnabled = b
		}
	}
	if v, ok := payload["tools.read_only"]; ok {
		if b, err := boolFromAny(v); err == nil {
			o.settings.ToolReadOnly = b
		}
	}
	if v, ok := payload["tools.timeout_seconds"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ToolTimeoutSeconds = n
		}
	}
	if v, ok := payload["tools.output_max_bytes"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ToolOutputMaxBytes = n
		}
	}
	if v, ok := payload["tools.absolute_max_bytes"]; ok {
		if n, err := intFromAny(v); err == nil {
			o.settings.ToolOutputAbsoluteMaxBytes = n
		}
	}

	// Wavefront: take effect on the next plan cycle.
	if v, ok := payload["wavefront.enabled"]; ok {
		if b, err := boolFromAny(v); err == nil {
			o.settings.WavefrontEnabled = b
		}
	}

	// --- Restart-required keys are silently ignored here; they are stored
	// in o.restartQueue by SetConfig. ---
}

// Restart stops the current orchestrator run and restarts it with the
// queued config changes (Phase 1e, PD-CONFIG-AF-009). Keys that required
// restart — provider, host, model, transport — are reapplied from the
// restartQueue before Start() rebuilds the model adapter and transport.
//
// Returns an error if no config was queued, the orchestrator is not started,
// or restart fails.
func (o *Orchestrator) ExecuteRestart() error {
	o.mu.Lock()
	q := o.restartQueue
	started := o.started
	o.mu.Unlock()

	if !started {
		return fmt.Errorf("orchestrator not started; cannot restart")
	}
	if q == nil {
		return fmt.Errorf("no restart-queued config changes")
	}

	// Shutdown the running session (stops transport, drains recorder).
	if err := o.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("shutdown before restart: %w", err)
	}

	// Apply the queued config to the settings, then re-Start().
	// decodeConfigPayload mutates a Config; we decode into a throwaway and
	// copy the fields into o.settings.
	var queued config.Config
	if err := decodeConfigPayload(q, &queued); err != nil {
		return fmt.Errorf("decode queued config: %w", err)
	}
	o.applyQueuedSettings(queued)

	// Re-start with the updated settings.
	if err := o.Start(); err != nil {
		return fmt.Errorf("restart Start: %w", err)
	}

	// Clear the queue on successful restart.
	o.mu.Lock()
	o.restartQueue = nil
	o.mu.Unlock()

	return nil
}

// applyQueuedSettings copies fields from a decoded config into o.settings.
// Called under o.mu during Restart().
func (o *Orchestrator) applyQueuedSettings(cfg config.Config) {
	o.settings.Provider = cfg.Provider()
	o.settings.OllamaHost = cfg.OllamaHost()
	o.settings.OllamaModel = cfg.OllamaModel()
	o.settings.LlamacppHost = cfg.LlamacppHost()
	o.settings.LlamacppModel = cfg.LlamacppModel()
}

// QueuedRestartKeys reports the config keys currently queued for restart
// (Phase 1e). Returns nil when no restart is pending. Used by the config
// surface to display the "requires restart" indicator next to pending keys.
func (o *Orchestrator) QueuedRestartKeys() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.restartQueue == nil {
		return nil
	}
	keys := make([]string, 0)
	for k := range o.restartQueue {
		keys = append(keys, k)
	}
	return keys
}

// HasQueuedRestart reports whether a restart is pending (Phase 1e).
func (o *Orchestrator) HasQueuedRestart() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.restartQueue != nil
}

// decodeConfigPayload decodes a flat JSON map into a typed config.Config.
// The transport layer sends keys as flat strings ("ollama_host") while the
// Config struct uses nested tables ([agentx.ollama] host). This function maps
// flat keys to the nested struct.
func decodeConfigPayload(payload map[string]any, cfg *config.Config) error {
	for k, v := range payload {
		s, ok := v.(string)
		if !ok {
			continue // non-string values (ints, bools) are handled by the TOML decoder
		}
		switch k {
		case "provider", "chat_backend":
			cfg.Agentx.Provider = s
			cfg.Agentx.ChatBackend = s
		case "ollama_host":
			cfg.Agentx.Ollama.Host = s
		case "ollama_model":
			cfg.Agentx.Ollama.Model = s
		case "llamacpp_host":
			cfg.Agentx.Llamacpp.Host = s
		case "llamacpp_model":
			cfg.Agentx.Llamacpp.Model = s
		case "classification.retries":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Classification.Retries = n
			}
		case "classification.clarification_options":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Classification.ClarificationOptions = n
			}
		case "output.max_widget_lines":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Output.MaxWidgetLines = n
			}
		case "output.input_max_lines":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Output.InputMaxLines = n
			}
		case "output.markdown_renderer":
			cfg.Agentx.Output.MarkdownRenderer = s
		case "thinking.enabled":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Thinking.Enabled = &b
			}
		case "thinking.time_budget_seconds":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Thinking.TimeBudgetSeconds = n
			}
		case "tools.enabled":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Tools.Enabled = &b
			}
		case "tools.read_only":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Tools.ReadOnly = &b
			}
		case "tools.timeout_seconds":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.TimeoutSeconds = n
			}
		case "tools.output_max_bytes":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.OutputMaxBytes = n
			}
		case "tools.absolute_max_bytes":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.AbsoluteMaxBytes = n
			}
		case "transport.enabled":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Transport.Enabled = &b
			}
		case "transport.host":
			cfg.Agentx.Transport.Host = s
		case "transport.port_start":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Transport.PortStart = n
			}
		case "transport.port_end":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Transport.PortEnd = n
			}
		case "wavefront.enabled":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Wavefront.Enabled = &b
			}
		}
	}
	return nil
}

// intFromAny extracts an int from a JSON number (float64) or string.
func intFromAny(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case string:
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
			return 0, err
		}
		return n, nil
	default:
		return 0, fmt.Errorf("not an int: %T", v)
	}
}

// boolFromAny extracts a bool from a JSON bool, string "true"/"false", or
// numeric 0/1.
func boolFromAny(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		s := strings.TrimSpace(strings.ToLower(val))
		switch s {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("not a bool: %q", val)
		}
	case float64:
		switch int(val) {
		case 0:
			return false, nil
		case 1:
			return true, nil
		default:
			return false, fmt.Errorf("not a bool: %v", val)
		}
	default:
		return false, fmt.Errorf("not a bool: %T", v)
	}
}
