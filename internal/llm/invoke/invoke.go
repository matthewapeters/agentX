// Package invoke provides the backend-agnostic fanout.Invoker: it turns a
// fanout.Invocation into a structured, schema-constrained model completion and
// parses the JSON back into a fanout.Response the pool can vote on.
//
// The model call is backed by a provider.Provider so the request-building and
// response-parsing logic is testable with a stub; NewProvider wires the
// production path over a real backend (Ollama, llama.cpp, …).
//
// The invoker reads the provider's FormatStyle once at construction. When the
// style is FormatStylePrompt, it injects a JSON instruction into the user prompt
// instead of sending a format field — so the rest of the runtime never sees the
// backend-specific difference.
//
// Design: docs/architecture/prompt_fan_groups.md, cascade_classifier.md, ADR 0013.
// Behavior contract: tests/features/llm/invoker.feature, tests/features/llm/provider.feature.
package invoke

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"agentx/internal/llm/fanout"
	"agentx/internal/llm/ollama"
	"agentx/internal/llm/provider"
)

// jsonInstruction is the system-style directive injected into the user prompt
// when a provider uses FormatStylePrompt (llama.cpp). It asks the model to
// emit a JSON object with the required fields and nothing else.
const jsonInstruction = "Respond with a JSON object using only the fields listed below. Do not include any prose or explanation outside the JSON object."

// Request is one model completion the invoker asks for.
//
// Deprecated: Request is kept for backward compatibility with existing test
// stubs that construct it directly. New code should use provider.CompleteRequest.
type Request struct {
	Model       string
	System      string
	User        string
	Temperature float64
	Seed        int
	Format      json.RawMessage // JSON schema for constrained decoding; nil = unconstrained or prompt-injected
}

// Invoker implements fanout.Invoker over a provider.Provider.
type Invoker struct {
	provider    provider.Provider
	formatStyle provider.FormatStyle
	model       string // default model when an invocation names none
	system      string // optional system framing prepended to every invocation
}

// NewProvider builds an Invoker backed by the given provider.
func NewProvider(model, system string, p provider.Provider) *Invoker {
	return &Invoker{
		provider:    p,
		formatStyle: p.FormatStyle(),
		model:       model,
		system:      system,
	}
}

// NewOllama wires the production invoker over an Ollama client, for backward
// compatibility with existing call sites. Equivalent to NewProvider with an
// Ollama provider (FormatStyleNative).
func NewOllama(client *ollama.Client, model, system string) *Invoker {
	return NewProvider(model, system, ollama.NewOllamaProvider(client))
}

// Invoke satisfies fanout.Invoker: it sends the invocation's prompt as a
// schema-constrained completion and parses the JSON reply into a Response.
//
// When the provider's FormatStyle is FormatStyleNative, the JSON schema is sent
// as the "format" field on the request. When it is FormatStylePrompt, a JSON
// instruction is prepended to the user prompt instead — the model must emit
// structured output via prompt engineering rather than a server-side hook.
func (i *Invoker) Invoke(ctx context.Context, inv fanout.Invocation) (fanout.Response, error) {
	model := inv.Model
	if model == "" {
		model = i.model
	}
	schema := schemaFor(inv.Contract)

	var userPrompt string
	var format json.RawMessage
	if schema != nil && i.formatStyle == provider.FormatStylePrompt {
		// Inject JSON instruction into the prompt — the provider does not honor
		// "format", so we ask the model via prompt engineering instead.
		userPrompt = jsonInstruction + "\n\n" + inv.Prompt
		format = nil
	} else {
		userPrompt = inv.Prompt
		format = schema
	}

	raw, err := i.provider.Complete(ctx, provider.CompleteRequest{
		Model:       model,
		Messages:    buildMessages(i.system, userPrompt),
		Temperature: inv.Params.Temperature,
		Seed:        inv.Params.Seed,
		Format:      format,
	})
	if err != nil {
		return fanout.Response{}, err
	}
	return parseResponse(raw, inv.VerdictField), nil
}

// buildMessages assembles system + user messages in the provider's Message
// representation. The provider's Message type is used so that the resulting
// payload matches its wire format exactly.
func buildMessages(system, user string) []provider.Message {
	msgs := make([]provider.Message, 0, 2)
	if system != "" {
		msgs = append(msgs, provider.Message{Role: "system", Content: system})
	}
	msgs = append(msgs, provider.Message{Role: "user", Content: user})
	return msgs
}

// schemaFor compiles a fanout.Contract into an Ollama `format` JSON schema so the
// model is constrained to emit the required fields. Returns nil when the contract
// requires no fields (unconstrained output).
func schemaFor(c fanout.Contract) json.RawMessage {
	if len(c.RequireFields) == 0 {
		return nil
	}
	props := make(map[string]any, len(c.RequireFields))
	for _, f := range c.RequireFields {
		if f == "confidence" {
			props[f] = map[string]any{"type": "number"}
		} else {
			props[f] = map[string]any{"type": "string"}
		}
	}
	schema := map[string]any{
		"type":       "object",
		"required":   c.RequireFields,
		"properties": props,
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// parseResponse maps a raw model reply into a fanout.Response. Malformed output
// (no parseable JSON object) yields an empty-Fields response, which the fold's
// contract check quarantines rather than counting — a junk reply never poisons a
// vote.
func parseResponse(raw, verdictField string) fanout.Response {
	resp := fanout.Response{Text: raw}
	var obj map[string]any
	if err := json.Unmarshal([]byte(extractJSON(raw)), &obj); err != nil {
		return resp
	}
	fields := make(map[string]string, len(obj))
	for k, v := range obj {
		fields[k] = stringify(v)
	}
	resp.Fields = fields
	if verdictField != "" {
		resp.Verdict = fields[verdictField]
	}
	switch c := obj["confidence"].(type) {
	case float64:
		resp.Confidence = c
	case string:
		if f, err := strconv.ParseFloat(c, 64); err == nil {
			resp.Confidence = f
		}
	}
	if ms, ok := obj["milestones"].([]any); ok {
		for _, m := range ms {
			resp.Milestones = append(resp.Milestones, stringify(m))
		}
	}
	return resp
}

// extractJSON pulls the first {...} object out of a reply, tolerating a local
// model that wraps its JSON in prose or code fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
