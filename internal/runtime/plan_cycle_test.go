package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"agentx/internal/executor"
	"agentx/internal/planfindings"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/wavefront"
	"agentx/internal/tools"
)

// stubTaskExec returns a fixed outcome regardless of the record.
type stubTaskExec struct{ out executor.Outcome }

func (s stubTaskExec) Execute(context.Context, task.Record) executor.Outcome { return s.out }

// stubArtifactReader maps a ref to its "full" stored content, mimicking the artifact store
// (whose Write always persists the complete output, independent of the tiny UI preview).
type stubArtifactReader map[string]string

func (r stubArtifactReader) Read(ref string, offset, limit int) ([]byte, error) {
	return []byte(r[ref]), nil
}

// TestCapturingExecWidensFindings reproduces the lively-raven bug directly: a result whose
// UI Preview is short (the executor's 20-line-style truncation) but whose full artifact
// content is long — findings must carry the WIDE content, not the narrow preview, or
// synthesis loses exactly the evidence (e.g. go.mod/cmd/ far down a tree listing) a short
// preview discards.
func TestCapturingExecWidensFindings(t *testing.T) {
	full := "00_START_HERE.md\nagentx.egg-info\n...\ngo.mod\ncmd/agentx/main.go\ninternal/\n"
	out := executor.Outcome{
		Status: executor.Executed,
		Result: tools.Result{Status: "ok", Preview: "00_START_HERE.md\nagentx.egg-info\n...", Ref: "ref-tree"},
	}
	c := &capturingExec{
		inner:  stubTaskExec{out: out},
		reader: stubArtifactReader{"ref-tree": full},
	}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "get full recursive tree"})

	if len(c.steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(c.steps))
	}
	got := c.steps[0].findings
	if !strings.Contains(got, "go.mod") || !strings.Contains(got, "cmd/agentx/main.go") {
		t.Fatalf("findings did not carry the widened content: %q", got)
	}

	ctx := planContext(c.steps)
	if !strings.Contains(ctx, "go.mod") {
		t.Errorf("planContext output lost the widened findings: %q", ctx)
	}
}

// TestCapturingExecFallsBackToPreview: no reader, or an unknown ref, degrades to the UI
// preview rather than losing the finding entirely.
func TestCapturingExecFallsBackToPreview(t *testing.T) {
	out := executor.Outcome{
		Status: executor.Executed,
		Result: tools.Result{Status: "ok", Preview: "short preview only", Ref: "ref-missing"},
	}
	c := &capturingExec{inner: stubTaskExec{out: out}} // reader left nil
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	if c.steps[0].findings != "short preview only" {
		t.Errorf("findings = %q, want the preview fallback", c.steps[0].findings)
	}
}

// TestCapturingExecNoRefFallsBackToPreview: a result with no Ref (e.g. a builtin that
// never wrote an artifact) also falls back cleanly.
func TestCapturingExecNoRefFallsBackToPreview(t *testing.T) {
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Status: "ok", Preview: "preview text"}}
	c := &capturingExec{inner: stubTaskExec{out: out}, reader: stubArtifactReader{}}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	if c.steps[0].findings != "preview text" {
		t.Errorf("findings = %q, want the preview fallback", c.steps[0].findings)
	}
}

// TestCapturingExecFindingsEmpty: nothing executed yet — Findings must return "" so
// planfindings.From (and thus decompose/proposal grounding) is a no-op, not an empty
// but present "[This plan's findings so far]" header.
func TestCapturingExecFindingsEmpty(t *testing.T) {
	c := &capturingExec{inner: stubTaskExec{out: executor.Outcome{}}}
	if got := c.Findings(); got != "" {
		t.Errorf("Findings on an empty plan = %q, want \"\"", got)
	}
}

// TestCapturingExecFindingsAccumulates: the amber-quartz fix's core mechanism — each
// completed leaf is visible to Findings immediately (live during the drain, not only
// after it ends), and a later read sees strictly more than an earlier one.
func TestCapturingExecFindingsAccumulates(t *testing.T) {
	c := &capturingExec{inner: stubTaskExec{out: executor.Outcome{Status: executor.Executed}}}
	if got := c.Findings(); got != "" {
		t.Fatalf("Findings before any step = %q, want \"\"", got)
	}

	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "list project root"})
	afterOne := c.Findings()
	if !strings.Contains(afterOne, "list project root") {
		t.Errorf("Findings after t1 = %q, want it to mention t1's goal", afterOne)
	}

	c.Execute(context.Background(), task.Record{ID: "t2", Goal: "run tree"})
	afterTwo := c.Findings()
	if !strings.Contains(afterTwo, "list project root") || !strings.Contains(afterTwo, "run tree") {
		t.Errorf("Findings after t2 = %q, want both t1 and t2 present", afterTwo)
	}
	if len(afterTwo) <= len(afterOne) {
		t.Errorf("Findings did not grow monotonically: after one=%d bytes, after two=%d bytes", len(afterOne), len(afterTwo))
	}
}

// TestCapturingExecFindingsCapsPerStepWidth: findings feed back into EVERY subsequent
// decompose/proposal call during the same drain, not read once — so each step's
// contribution is capped much tighter (midDrainFindingsLines) than the one-shot final
// synthesis budget (findingsLines).
func TestCapturingExecFindingsCapsPerStepWidth(t *testing.T) {
	var lines []string
	for i := 0; i < midDrainFindingsLines+10; i++ {
		lines = append(lines, "line")
	}
	wide := strings.Join(lines, "\n")
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: wide}}
	c := &capturingExec{inner: stubTaskExec{out: out}}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "wide result"})

	got := c.Findings()
	if strings.Count(got, "line") != midDrainFindingsLines {
		t.Errorf("Findings included %d of the wide result's lines, want exactly %d (capped)", strings.Count(got, "line"), midDrainFindingsLines)
	}
	if !strings.Contains(got, "…") {
		t.Error("truncated findings should be marked with an ellipsis")
	}
}

// TestWithComposedFindingsNoPriorSource: the common case (no nested plan phase) —
// composition is a no-op, ctx just carries cap's own findings, unchanged from
// before this fix.
func TestWithComposedFindingsNoPriorSource(t *testing.T) {
	c := &capturingExec{inner: stubTaskExec{out: executor.Outcome{Status: executor.Executed}}}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "round one step"})

	ctx := withComposedFindings(context.Background(), c)
	if got := planfindings.From(ctx); !strings.Contains(got, "round one step") {
		t.Errorf("From = %q, want it to mention round one's step", got)
	}
}

// TestWithComposedFindingsWithPriorSource: the verb-continuation case — an outer
// (earlier round's) findings source, already attached to ctx, is composed UNDER the
// new cap's own findings, so a continuation round is grounded in both rounds, not
// just its own.
func TestWithComposedFindingsWithPriorSource(t *testing.T) {
	outerCtx := planfindings.WithSource(context.Background(), func() string {
		return "[round one] tree the project → done: cmd/agentx/main.go"
	})
	c := &capturingExec{inner: stubTaskExec{out: executor.Outcome{Status: executor.Executed}}}
	c.Execute(context.Background(), task.Record{ID: "t2", Goal: "round two step"})

	ctx := withComposedFindings(outerCtx, c)
	got := planfindings.From(ctx)
	if !strings.Contains(got, "round one") || !strings.Contains(got, "cmd/agentx/main.go") {
		t.Errorf("From = %q, want round one's findings still present", got)
	}
	if !strings.Contains(got, "round two step") {
		t.Errorf("From = %q, want round two's own findings present too", got)
	}
}

// TestCapturingExecBelowThresholdSkipsSummarizer: the common case (ADR 0012 §6) —
// findings under wavefront.OutputSummaryThreshold are stored as-is, with no
// summarizer call at all, so a tracked summarizer surfaces as never having been
// invoked.
func TestCapturingExecBelowThresholdSkipsSummarizer(t *testing.T) {
	var calls int
	summarize := func(ctx context.Context, chain []string, text string) string {
		calls++
		return "should not be called"
	}
	small := strings.Repeat("x", wavefront.OutputSummaryThreshold-1)
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: small}}
	c := &capturingExec{inner: stubTaskExec{out: out}, rootGoal: "g", summarize: summarize}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	if calls != 0 {
		t.Errorf("summarize called %d times, want 0 for below-threshold findings", calls)
	}
	if c.steps[0].findings != small {
		t.Error("below-threshold findings must be stored verbatim")
	}
}

// TestCapturingExecAboveThresholdSummarizes: oversized findings trigger exactly one
// summarizer call, chain-aware, and its result (already disclosed/condensed by
// wavefront.CondenseFunc) is stored as-is (ADR 0012 §6).
func TestCapturingExecAboveThresholdSummarizes(t *testing.T) {
	var gotChain []string
	var gotText string
	summarize := func(ctx context.Context, chain []string, text string) string {
		gotChain = chain
		gotText = text
		return "[summarized from N chars] condensed version"
	}
	big := strings.Repeat("x", wavefront.OutputSummaryThreshold+1)
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: big}}
	c := &capturingExec{inner: stubTaskExec{out: out}, rootGoal: "top-level goal", summarize: summarize}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "specific step goal"})

	if gotText != big {
		t.Errorf("summarize received %d chars, want the full %d-char findings", len(gotText), len(big))
	}
	if want := []string{"top-level goal", "specific step goal"}; !slices.Equal(gotChain, want) {
		t.Errorf("chain = %v, want %v (general to specific)", gotChain, want)
	}
	got := c.steps[0].findings
	if got != "[summarized from N chars] condensed version" {
		t.Errorf("findings = %q, want the summarizer's result stored verbatim", got)
	}
}

// TestCapturingExecRootStepCollapsesChainToOneItem: when the step IS the root
// question, the chain must not repeat the same goal twice.
func TestCapturingExecRootStepCollapsesChainToOneItem(t *testing.T) {
	var gotChain []string
	summarize := func(ctx context.Context, chain []string, text string) string {
		gotChain = chain
		return "condensed"
	}
	big := strings.Repeat("x", wavefront.OutputSummaryThreshold+1)
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: big}}
	c := &capturingExec{inner: stubTaskExec{out: out}, rootGoal: "same goal", summarize: summarize}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "same goal"})

	if want := []string{"same goal"}; !slices.Equal(gotChain, want) {
		t.Errorf("chain = %v, want %v (collapsed, not repeated)", gotChain, want)
	}
}

// TestCapturingExecNilSummarizerFallsBackToTruncation: no summarizer wired at all
// (the degrade-gracefully posture already established for a nil artifactReader)
// falls back to wavefront.TruncateFindings directly — a failing/empty summarizer
// call is wavefront.NewCondenser's own responsibility now (see
// internal/runtime/wavefront/summarize_test.go), not capturingExec's.
func TestCapturingExecNilSummarizerFallsBackToTruncation(t *testing.T) {
	big := strings.Repeat("x", wavefront.OutputSummaryThreshold+1)
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: big}}
	c := &capturingExec{inner: stubTaskExec{out: out}, rootGoal: "g"} // summarize left nil
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	got := c.steps[0].findings
	if !strings.Contains(got, "[truncated,") {
		t.Errorf("findings = %q, want a disclosed truncation fallback", got)
	}
}

// TestCapturingExecFindingsConcurrentWithExecute mirrors the scheduler's real usage:
// Execute runs on worker goroutines that may be mid-append while a decompose/proposal
// call on another goroutine reads Findings for the SAME plan. Run with -race.
func TestCapturingExecFindingsConcurrentWithExecute(t *testing.T) {
	c := &capturingExec{inner: stubTaskExec{out: executor.Outcome{Status: executor.Executed}}}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Execute(context.Background(), task.Record{ID: fmt.Sprintf("t%d", i), Goal: fmt.Sprintf("leaf %d", i)})
		}(i)
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				_ = c.Findings()
			}
		}
	}()
	wg.Wait()
	close(done)

	if got := c.Findings(); strings.Count(got, "leaf ") != 20 {
		t.Errorf("Findings after all leaves completed mentions %d leaves, want 20", strings.Count(got, "leaf "))
	}
}
