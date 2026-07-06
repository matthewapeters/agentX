// Package invoke provides the Ollama-backed fanout.Invoker: it turns a
// fanout.Invocation into a structured, schema-constrained model completion and
// parses the JSON back into a fanout.Response the pool can vote on.
//
// The model call is injected as a CompleteFunc so the request-building and
// response-parsing logic is testable without a live Ollama; NewOllama wires the
// production path over an *ollama.Client.
//
// Design: docs/architecture/prompt_fan_groups.md, cascade_classifier.md.
// Behavior contract: tests/features/llm/invoker.feature.
package invoke

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"agentx/internal/llm/fanout"
	"agentx/internal/llm/ollama"
)

// Request is one model completion the invoker asks for.
type Request struct {
	Model       string
	System      string
	User        string
	Temperature float64
	Seed        int
	Format      json.RawMessage // JSON schema for constrained decoding; nil = unconstrained
}

// CompleteFunc runs one completion and returns the raw model text.
type CompleteFunc func(ctx context.Context, req Request) (string, error)

// Invoker implements fanout.Invoker over a CompleteFunc.
type Invoker struct {
	complete CompleteFunc
	model    string // default model when an invocation names none
	system   string // optional system framing prepended to every invocation
}

// New builds an Invoker over a completion function.
func New(model, system string, complete CompleteFunc) *Invoker {
	return &Invoker{complete: complete, model: model, system: system}
}

// NewOllama wires the production invoker over an Ollama client.
func NewOllama(client *ollama.Client, model, system string) *Invoker {
	return New(model, system, func(ctx context.Context, r Request) (string, error) {
		msgs := make([]ollama.Message, 0, 2)
		if r.System != "" {
			msgs = append(msgs, ollama.Message{Role: "system", Content: r.System})
		}
		msgs = append(msgs, ollama.Message{Role: "user", Content: r.User})
		return client.Complete(ctx, ollama.CompleteRequest{
			Model:       r.Model,
			Messages:    msgs,
			Temperature: r.Temperature,
			Seed:        r.Seed,
			Format:      r.Format,
		})
	})
}

// Invoke satisfies fanout.Invoker: it sends the invocation's prompt as a
// schema-constrained completion and parses the JSON reply into a Response.
func (i *Invoker) Invoke(ctx context.Context, inv fanout.Invocation) (fanout.Response, error) {
	model := inv.Model
	if model == "" {
		model = i.model
	}
	raw, err := i.complete(ctx, Request{
		Model:       model,
		System:      i.system,
		User:        inv.Prompt,
		Temperature: inv.Params.Temperature,
		Seed:        inv.Params.Seed,
		Format:      schemaFor(inv.Contract),
	})
	if err != nil {
		return fanout.Response{}, err
	}
	return parseResponse(raw, inv.VerdictField), nil
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
