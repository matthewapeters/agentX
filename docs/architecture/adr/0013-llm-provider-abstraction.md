# ADR 0013: LLM Provider Abstraction

Status: Accepted
Date: 2026-07-28
Deciders: AgentX architecture owners

## Context

AgentX v1 hard-codes Ollama as its only LLM backend. The `internal/llm/ollama` package is wired into `invoke.Invoker`, `fanout.Invoker`, `runtime.Model`, and the config layer. A user who wants to switch to (or run alongside) llama.cpp must fork the project — every call site assumes Ollama's HTTP endpoints, parameter names, and JSON-schema `format` field.

Two local backends are in play today (Ollama and llama.cpp). More will follow (vLLM, OpenAI-compatible clouds). A provider abstraction now prevents each new backend from touching every call site.

## Decision

Introduce a `internal/llm/provider.Provider` interface that both Ollama and llama.cpp implement. The interface captures exactly what the rest of the runtime needs:

```go
// Message is a single chat message (role + content).
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// CompleteRequest is a non-streaming chat completion. Format carries a JSON
// schema when the provider honors native constrained decoding; nil when the
// invoker injects the constraint into the prompt instead.
type CompleteRequest struct {
    Model       string
    Messages    []Message
    Temperature float64
    Seed        int
    Format      json.RawMessage // nil = unconstrained
    FormatStyle FormatStyle     // controls how Format is honored
    NumCtx      int
    Think       bool
}

// ChatRequest is a streaming chat completion.
type ChatRequest struct {
    Model    string
    Messages []Message
    Think    bool
    NumCtx   int
}

// FormatStyle controls how the invoker asks the model to constrain output.
type FormatStyle int
const (
    FormatStyleNone   FormatStyle = iota // no constraint; Format must be nil
    FormatStyleNative                    // provider honors "format" natively (Ollama)
    FormatStylePrompt                    // provider does NOT honor "format";
                                         // invoker injects JSON instruction into
                                         // the user prompt (llama.cpp)
)

// Provider is the LLM backend seam. Every provider speaks chat-completion HTTP;
// the shape of each request and the meaning of each field are provider-specific,
// but the surface contract is uniform across implementations.
type Provider interface {
    // FormatStyle reports how this provider handles Constrained decoding.
    FormatStyle() FormatStyle

    // Complete runs a single non-streaming chat completion.
    Complete(ctx context.Context, req CompleteRequest) (string, error)

    // Chat streams a chat completion. onDelta fires for each content chunk;
    // onThink (when non-nil and Think is set) fires for each reasoning chunk.
    Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (string, error)

    // Ready reports whether the host is reachable and the model is available.
    Ready(ctx context.Context, model string) error

    // ContextLength reports the model's maximum context window in tokens.
    ContextLength(ctx context.Context, model string) (int, error)
}
```

### Why this shape

- **`FormatStyle` on the request, not the interface.** Each provider declares its own style (`Native` for Ollama, `Prompt` for llama.cpp). The invoker reads it once at construction and adjusts its behavior — injecting JSON into the prompt when style is `Prompt`, passing `Format` through when style is `Native`. This is the single behavioral difference between backends at the invocation layer.
- **`FormatStyle` is on `Provider`, not `CompleteRequest`.** The invoker reads it once per provider at construction and never re-checks. It is not a per-request knob — it is a property of the backend.
- **`Message` lives on the provider package.** Each provider defines its own `Message` type (matching the wire format of its backend). The `invoke` package converts `prompting.Message` → provider-specific `Message` once at the adapter boundary. The `fanout` pool never sees `Message`.
- **`Complete` and `Chat` are the only streaming seams.** `Ready` and `ContextLength` are bootstrap probes; their implementation is trivially provider-specific.

### Wiring in `invoke.Invoker`

The invoker becomes provider-agnostic. It reads the provider's `FormatStyle` once at construction:

```go
type Invoker struct {
    provider  provider.Provider
    formatStyle provider.FormatStyle
    model     string
    system    string
}

func NewProviderBacked(model, system string, p provider.Provider) *Invoker {
    return &Invoker{
        provider:    p,
        formatStyle: p.FormatStyle(),
        model:       model,
        system:      system,
    }
}
```

When `FormatStyle == FormatStylePrompt`, the invoker injects a JSON instruction into the user prompt instead of sending the schema as `Format`. When `FormatStyle == FormatStyleNative`, it passes `Format` through unchanged. Call sites (classifier, planner, decomposition) see no difference.

### Wiring in `runtime.Model`

The `Model` interface stays the same. Two adapters wrap `Provider`:

```go
type ollamaModel struct { provider.Provider }
type llamacppModel struct { provider.Provider }
```

Both delegate to their wrapped provider. `newOllamaModel` and `newLlamacppModel` build the adapter from the configured host + model. The orchestrator selects the adapter by `config.Provider()`.

### Config

`agentx.toml` gains a `provider` key (`"ollama"` or `"llamacpp"`) at the `[agentx]` table level. The existing `[agentx.ollama]` table is reused as `[agentx.<provider>]` — both providers share `host` and `model` keys. A provider-specific section (e.g. API key for llama.cpp) can be added later without breaking backward compat.

```toml
[agentx]
provider = "llamacpp"        # "ollama" or "llamacpp"

[agentx.llamacpp]
host = "localhost:8080"
model = "ornith-1.0-35b-Q4_K_M"
```

### Backward compatibility

Existing `agentx.toml` files with `provider = "ollama"` (or no `provider` key) behave identically — the default resolves to Ollama. The `chat_backend` key (mentioned in `docs/ux/03_PANEL_DETAILS.md`) is retained as an alias for `provider` for users who already set it.

### What does NOT change

- `internal/llm/fanout` — the pool, `Invocation`, `Response`, `Aggregator`, `Decision` — unchanged.
- `internal/llm/invoke` behavior — parsing, JSON extraction, quarantine — unchanged.
- `runtime.Orchestrator` orchestration — unchanged; only the model adapter construction differs.
- `runtime.Settings` — gains `Provider` (string) and a new `WithProvider` option replaces the old `OllamaHost`/`OllamaModel` fields. Existing tests inject a `Model` via `WithModel(stub)` and are unaffected.

### Open questions

1. **llama.cpp server vs. native cgo.** This ADR targets llama.cpp's HTTP server mode only. A future ADR may cover native cgo bindings for in-process inference; the `Provider` interface already accommodates that path.
2. **Per-provider timeouts.** Not yet exposed — both providers use the orchestrator's thinking budget. A future ADR may add per-provider timeout config.
3. **Provider-specific options.** `NumCtx`, `Think`, `Seed`, `Temperature` are shared. Provider-specific sampling parameters (e.g. llama.cpp's `top_k`, `top_p`, `repeat_penalty`) can be added to `CompleteRequest` as future ADRs.

## Consequences

- **Adding a third provider** (vLLM, OpenAI, …) is a 1–2 day effort: implement `Provider`, add one adapter to `runtime.Model`, dispatch in the orchestrator. Zero changes to `fanout`, `invoke`, or orchestration.
- **`FormatStyle` drives the only behavioral divergence.** The invoker is the single place that cares whether a provider supports native JSON-schema; prompt-injection is automatic.
- **Config surface grows by one key.** `provider` defaults to `"ollama"`. No migration needed for existing deployments.
- **Test footprint.** Each provider gets its own hermetic feature file + step package. The invoker feature file gains two scenarios (native vs. prompt-style formatting) driven by a shared invoker world over two different provider stubs.

## References

- ADR 0001 — Orchestrator control/execution/data planes (seams)
- `tests/features/llm/invoker.feature` — invoker behavior
- `tests/features/llm/provider.feature` — provider abstraction behavior (this ADR)
- `tests/features/llm/llamacpp_adapter.feature` — llama.cpp adapter behavior (this ADR)
- `docs/implementation/04_llm_prompt_tooling_runtime.md` — default model service
