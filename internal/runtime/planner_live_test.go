package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"agentx/internal/config"
	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/decompose"
	"agentx/internal/tools"
)

// TestLivePlannerSynthesisDependsOnPriorNodes drives the REAL planner prompt —
// planner.DefaultPromptTemplate, the in-repo constant kept byte-identical to
// config/seed/agentx-planner.md (see that constant's doc comment) — against a
// live Ollama instance, repeatedly, for the "review the project" goal that
// surfaced a real bug: the model's plan put a synthesis/compile node in the
// DAG with no (or insufficient) "deps" on the data-gathering nodes it
// actually needs, so the scheduler dispatched it before its inputs existed
// and it failed.
//
// Deliberately does NOT read the user's live ~/.config/agentx/agentx-planner.md —
// that file is the user's own editable copy (seeded once, then theirs), and
// this test must never overwrite or depend on it. Iterate on wording purely
// in-repo (config/seed/agentx-planner.md + planner.DefaultPromptTemplate, kept
// in sync) and rerun; the user re-seeds their own install when ready.
//
// Opt-in only — hits a live LLM, is slow (each run is a real inference call),
// and is inherently non-deterministic (that's the point: it measures
// reliability across samples, not a single pass/fail). Set AGENTX_LIVE_TEST=1
// to run it:
//
//	AGENTX_LIVE_TEST=1 go test ./internal/runtime/ -run TestLivePlannerSynthesisDependsOnPriorNodes -v -count=1
//
// AGENTX_LIVE_RUNS overrides the sample count (default 5).
func TestLivePlannerSynthesisDependsOnPriorNodes(t *testing.T) {
	if os.Getenv("AGENTX_LIVE_TEST") != "1" {
		t.Skip("opt-in live-LLM test; set AGENTX_LIVE_TEST=1 to run")
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatalf("config.DefaultPaths: %v", err)
	}
	cfg, _, err := config.Resolve(paths)
	if err != nil {
		t.Fatalf("config.Resolve: %v", err)
	}

	client := ollama.New(cfg.OllamaHost())
	model := cfg.OllamaModel()
	chat := func(ctx context.Context, prompt string, format json.RawMessage) (string, error) {
		return client.Complete(ctx, ollama.CompleteRequest{
			Model:    model,
			Messages: []ollama.Message{{Role: "user", Content: prompt}},
			Format:   format,
		})
	}
	// Empty Template: LLMPlanner falls back to planner.DefaultPromptTemplate.
	p := decompose.LLMPlanner{Chat: chat, Catalog: plannerCatalog(tools.DefaultRegistry())}

	const goal = "review the current project. Identify five features you feel are well done, and five features that need additional work."

	runs := 5
	if n := os.Getenv("AGENTX_LIVE_RUNS"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			runs = v
		}
	}

	passed, failed, noAggregation := 0, 0, 0
	for i := 1; i <= runs; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		plan, err := p.Plan(ctx, "livetest", goal, "")
		cancel()
		if err != nil {
			t.Logf("run %d/%d: Plan error: %v", i, runs, err)
			continue
		}
		t.Logf("run %d/%d: plan %q — %d nodes", i, runs, plan.Name, len(plan.Records))
		for _, r := range plan.Records {
			t.Logf("  %s [%s] deps=%v goal=%q", r.ID, r.Kind, r.Deps, r.Goal)
			if isUnpromptedWrite(r) {
				t.Logf("    NOTE: this node writes to disk (%v) though the goal never asked to persist anything", r.Params)
			}
		}
		agg := findAggregationNode(plan.Records)
		switch {
		case agg == nil:
			noAggregation++
			t.Logf("run %d/%d: no aggregation-shaped node this round (fine — recursion may add one later)", i, runs)
		case aggregationDepsAreSufficient(*agg, plan.Records):
			passed++
			t.Logf("run %d/%d: PASS — %q correctly depends on its data-gathering siblings", i, runs, agg.ID)
		default:
			failed++
			t.Errorf("run %d/%d: FAIL — %q (goal %q) has insufficient deps %v for its %d siblings; see log above", i, runs, agg.ID, agg.Goal, agg.Deps, len(plan.Records)-1)
		}
	}
	t.Logf("summary: %d pass / %d fail / %d had no aggregation node, of %d runs", passed, failed, noAggregation, runs)
}

// aggregationKeywords flags a node whose own stated goal/description reasons
// about or combines OTHER nodes' results — a synthesis, comparison, or
// report-writing step — which is exactly the shape that must declare deps on
// the siblings it draws from.
var aggregationKeywords = []string{
	"compile", "combine", "synthesiz", "synthes", "summar", "aggregat", "consolidat", "report",
}

// findAggregationNode returns the first record whose Goal text matches an
// aggregation keyword, or nil if this plan has none at this level.
func findAggregationNode(records []task.Record) *task.Record {
	for i, r := range records {
		low := strings.ToLower(r.Goal)
		for _, kw := range aggregationKeywords {
			if strings.Contains(low, kw) {
				return &records[i]
			}
		}
	}
	return nil
}

// aggregationDepsAreSufficient reports whether agg declares deps on at least
// half of its sibling nodes. An aggregation node with zero deps (the bug this
// test targets) or one that only depends on a single sibling out of several
// data-gathering nodes is still under-specified.
func aggregationDepsAreSufficient(agg task.Record, records []task.Record) bool {
	others := len(records) - 1
	if others <= 0 {
		return true
	}
	if len(agg.Deps) == 0 {
		return false
	}
	return len(agg.Deps) >= (others+1)/2
}

// isUnpromptedWrite flags a task node whose tool mutates disk state
// (write_file) — worth surfacing separately since the reproduction that
// motivated this test involved the model inventing a "write the report to
// review.md" task the user's goal never asked for.
func isUnpromptedWrite(r task.Record) bool {
	tool, _ := r.Params["tool"].(string)
	return tool == "write_file"
}

