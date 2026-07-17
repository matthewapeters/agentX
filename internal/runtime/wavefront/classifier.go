package wavefront

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"agentx/internal/jsonx"
)

// Know is a fact already true, or derivable purely by combining facts already known
// — never a guess.
type Know struct{ Name, Value string }

// Command is a resolved tool call, matching the existing planner's task payload
// shape ({"tool","args"}) — the same catalog/executor path handles both engines'
// resolved calls identically; there is no separate "raw shell command" concept.
type Command struct {
	Tool string
	Args map[string]string
}

// Need is something required but not yet known. A non-nil Command resolves it
// immediately via the executor; nil makes it a new open question — a child node
// dispatched again later. Command.Args must come from the working-memory text
// Classify was given, never from a fact expected from a sibling, not-yet-executed
// Need — the core grounding rule ADR 0012 exists to enforce. This is the
// contract-level statement of that rule, not a runtime check: enforcement is prompt
// discipline (the classify prompt states it explicitly) plus the fact that nothing
// downstream binds an unresolved value into Args.
type Need struct {
	Name    string
	Command *Command
}

// Result is one Classify call's KNOW/NEED classification of a single question.
type Result struct {
	Knows []Know
	Needs []Need
}

// Classifier asks, for one open question against the current blackboard (rendered
// working-memory text — the graph itself, per the ADR 0012 amendment, not a
// separate struct), what's already known/synthesizable and what's still needed.
// Unlike planner.Planner.Plan, a Need's Command — when present — must be answerable
// from wm alone.
type Classifier interface {
	Classify(ctx context.Context, wm, question string) (Result, error)
}

// Wire shapes for the model's JSON reply.

type knowPayload struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type commandPayload struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

type needPayload struct {
	Name    string          `json:"name"`
	Command *commandPayload `json:"command,omitempty"`
}

// classifyItem is a discriminated union by key presence (JSON oneOf), mirroring
// planner.dagNode: exactly one of Know/Need is set — a nil pointer means absent,
// not a null/empty sentinel value.
type classifyItem struct {
	Know *knowPayload `json:"KNOW,omitempty"`
	Need *needPayload `json:"NEED,omitempty"`
}

type classifyJSON struct {
	Classification []classifyItem `json:"classification"`
}

// ClassifySchema is the JSON Schema handed to Ollama's Format field to constrain
// generation to the oneOf(KNOW,NEED) wire shape, with NEED's command optional (its
// absence is what makes a NEED an open question). A fixed schema, built once, same
// convention as planner.PlanSchema.
func ClassifySchema() json.RawMessage {
	return json.RawMessage(classifySchemaJSON)
}

const classifySchemaJSON = `{
  "type": "object", "required": ["classification"],
  "properties": {"classification": {"type": "array", "items": {"type": "object",
    "oneOf": [
      {"required": ["KNOW"], "properties": {
        "KNOW": {"type": "object", "required": ["name", "value"],
          "properties": {"name": {"type": "string", "minLength": 1}, "value": {"type": "string"}}}}},
      {"required": ["NEED"], "properties": {
        "NEED": {"type": "object", "required": ["name"],
          "properties": {
            "name": {"type": "string", "minLength": 1},
            "command": {"type": "object", "required": ["tool", "args"],
              "properties": {"tool": {"type": "string"}, "args": {"type": "object"}}}
          }}}}
    ]}}}
}`

// Parse turns the model's classify JSON into a Result. Rejects an item with both or
// neither of KNOW/NEED, an empty name on either, and a NEED whose command is present
// but missing a tool — failing loudly rather than emitting a partially broken
// classification. These checks are a defensive backstop: ClassifySchema's
// constrained decoding should prevent most of them, but Parse must not trust that
// blindly (mirrors planner.Parse's own posture) — a model/Ollama version can still
// fall back to unconstrained text, recovered via jsonx.FirstObject below.
func Parse(data []byte) (Result, error) {
	obj := jsonx.FirstObject(string(data))
	if obj == "" {
		return Result{}, fmt.Errorf("wavefront: no JSON object in classify response")
	}
	var cj classifyJSON
	if err := json.Unmarshal([]byte(obj), &cj); err != nil {
		return Result{}, fmt.Errorf("wavefront: parse classify json: %w", err)
	}

	var res Result
	for i, item := range cj.Classification {
		if (item.Know == nil) == (item.Need == nil) {
			return Result{}, fmt.Errorf("wavefront: item %d must have exactly one of KNOW or NEED", i+1)
		}
		switch {
		case item.Know != nil:
			name := strings.TrimSpace(item.Know.Name)
			if name == "" {
				return Result{}, fmt.Errorf("wavefront: KNOW item %d has no name", i+1)
			}
			res.Knows = append(res.Knows, Know{Name: name, Value: item.Know.Value})
		case item.Need != nil:
			name := strings.TrimSpace(item.Need.Name)
			if name == "" {
				return Result{}, fmt.Errorf("wavefront: NEED item %d has no name", i+1)
			}
			need := Need{Name: name}
			if item.Need.Command != nil {
				tool := strings.TrimSpace(item.Need.Command.Tool)
				if tool == "" {
					return Result{}, fmt.Errorf("wavefront: NEED %q has a command with no tool", name)
				}
				args := make(map[string]string, len(item.Need.Command.Args))
				maps.Copy(args, item.Need.Command.Args)
				need.Command = &Command{Tool: tool, Args: args}
			}
			res.Needs = append(res.Needs, need)
		}
	}
	return res, nil
}
