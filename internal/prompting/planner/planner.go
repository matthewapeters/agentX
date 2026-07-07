// Package planner is the decomposition planner of ADR 0008 Phase 4b: it renders the
// prompt that asks the model to break a compound goal into a plan, and parses the model's
// plan JSON into child task records with dependency edges.
//
// Unlike the fan-groups in the prompt corpus, the planner GENERATES a structured plan
// rather than voting on a single field, so it is not a corpus fan-group. The prompt lives
// here as a tunable template; it may move to config if the corpus format grows a generator
// block. The parser is the deterministic, LLM-free core exercised by the Phase-4b tests.
//
// Child ids are namespaced under the parent (parentID-1, parentID-2, …) so they are
// session-unique (the Phase-1 DAG requires it), and the model's local step ids are remapped
// onto them so dependency edges survive.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md.
// Behavior: docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md.
package planner

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentx/internal/prompting/task"
)

// PromptTemplate asks the model to decompose a goal into an independent-where-possible
// plan. {{goal}} is the compound goal; {{context}} is the branch's investigation context
// (working memory + read-tool findings). Tuned against the canonical fixture in Phase 4e.
const PromptTemplate = `You are planning how to accomplish a compound goal by breaking it
into concrete sub-steps. Prefer INDEPENDENT steps (no dependency) so they can run in
parallel; add a dependency only when one step truly needs another's result first.

Goal:
{{goal}}

What you know:
{{context}}

Reply with JSON only:
{"plan_name": "<3-6 words>", "synthesis": "<one sentence>", "steps": [
  {"id": "s1", "goal": "<a single concrete sub-step>", "deps": [], "cost": 1}
]}
Each step id is a short local label; deps lists the ids of steps that must finish first;
cost is a rough 1-5 effort estimate. Keep steps atomic — one action each.`

// Plan is a parsed decomposition: the child task records (DAG-shaped, with deps rewritten
// to the namespaced child ids), plus the model's plan name and synthesis.
type Plan struct {
	Name      string
	Synthesis string
	Records   []task.Record
}

// step is one entry of the model's plan JSON.
type step struct {
	ID   string   `json:"id"`
	Goal string   `json:"goal"`
	Deps []string `json:"deps"`
	Cost int      `json:"cost"`
}

type planJSON struct {
	Name      string `json:"plan_name"`
	Synthesis string `json:"synthesis"`
	Steps     []step `json:"steps"`
}

// Render fills the prompt template for a goal and its investigation context.
func Render(goal, context string) string {
	r := strings.NewReplacer("{{goal}}", goal, "{{context}}", context)
	return r.Replace(PromptTemplate)
}

// Parse turns the model's plan JSON into child task records under parentID. It rewrites the
// model's local step ids to session-unique child ids (parentID-1, parentID-2, …) and
// remaps deps onto them, so the returned records form a well-formed sub-DAG. It rejects an
// empty plan, a step missing an id or goal, a duplicate id, or a dep referencing an unknown
// step (a dangling edge) — failing loudly rather than emitting a broken plan.
func Parse(parentID string, data []byte) (Plan, error) {
	var pj planJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return Plan{}, fmt.Errorf("planner: parse plan json: %w", err)
	}
	if len(pj.Steps) == 0 {
		return Plan{}, fmt.Errorf("planner: plan has no steps")
	}

	// Map each local step id to its namespaced child id, rejecting missing/duplicate ids.
	idFor := make(map[string]string, len(pj.Steps))
	for i, s := range pj.Steps {
		local := strings.TrimSpace(s.ID)
		if local == "" {
			return Plan{}, fmt.Errorf("planner: step %d has no id", i+1)
		}
		if strings.TrimSpace(s.Goal) == "" {
			return Plan{}, fmt.Errorf("planner: step %q has no goal", local)
		}
		if _, dup := idFor[local]; dup {
			return Plan{}, fmt.Errorf("planner: duplicate step id %q", local)
		}
		idFor[local] = fmt.Sprintf("%s-%d", parentID, i+1)
	}

	records := make([]task.Record, 0, len(pj.Steps))
	for _, s := range pj.Steps {
		deps := make([]string, 0, len(s.Deps))
		for _, d := range s.Deps {
			mapped, ok := idFor[strings.TrimSpace(d)]
			if !ok {
				return Plan{}, fmt.Errorf("planner: step %q depends on unknown step %q", s.ID, d)
			}
			deps = append(deps, mapped)
		}
		rec := task.Record{
			ID:     idFor[strings.TrimSpace(s.ID)],
			Goal:   s.Goal,
			Type:   task.Query, // placeholder: the oracle classifies the real type per leaf
			Status: task.Proposed,
			Deps:   deps,
			Provenance: task.Provenance{
				Source: "planner",
				Spread: nil,
			},
		}
		if s.Cost > 0 {
			rec.Params = map[string]any{"cost": s.Cost}
		}
		records = append(records, rec)
	}

	return Plan{Name: pj.Name, Synthesis: pj.Synthesis, Records: records}, nil
}
