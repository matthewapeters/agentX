package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"agentx/internal/executor"
	"agentx/internal/planfindings"
	"agentx/internal/prompting/task"
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

// TestIncompleteNodesAllDoneIsClean: a fully-done plan reports zero across the board and
// no examples — the happy path confirmPlanIncomplete uses to skip the clarify prompt.
func TestIncompleteNodesAllDoneIsClean(t *testing.T) {
	recs := []task.Record{
		{ID: "a", Goal: "a", Status: task.Done},
		{ID: "b", Goal: "b", Status: task.Done},
	}
	failed, abstained, neverRan, examples := incompleteNodes(recs)
	if failed != 0 || abstained != 0 || neverRan != 0 || len(examples) != 0 {
		t.Fatalf("incompleteNodes(all done) = (%d,%d,%d,%v), want all zero", failed, abstained, neverRan, examples)
	}
}

// TestIncompleteNodesCountsAndExamples reproduces the witty-falcon shape: one abstained
// leaf blocking a chain of never-ran ancestors, and the abstained goal surfaced as an
// example (not just a count) so the clarify prompt can name what got stuck.
func TestIncompleteNodesCountsAndExamples(t *testing.T) {
	recs := []task.Record{
		{ID: "root", Goal: "root goal", Status: task.Proposed},
		{ID: "a", Goal: "a", Status: task.Done},
		{ID: "b", Goal: "check remaining .github files", Status: task.Abstained},
		{ID: "c", Goal: "write the file", Status: task.Proposed},
		{ID: "d", Goal: "d", Status: task.Failed},
	}
	failed, abstained, neverRan, examples := incompleteNodes(recs)
	if failed != 1 || abstained != 1 || neverRan != 2 {
		t.Fatalf("incompleteNodes = (failed=%d, abstained=%d, neverRan=%d), want (1,1,2)", failed, abstained, neverRan)
	}
	if len(examples) != 1 || examples[0] != "check remaining .github files" {
		t.Errorf("examples = %v, want the abstained node's goal", examples)
	}
}

// TestIncompleteNodesExamplesCapped: the clarify prompt stays compact even for a wide
// plan with many abstained leaves.
func TestIncompleteNodesExamplesCapped(t *testing.T) {
	var recs []task.Record
	for i := range 10 {
		recs = append(recs, task.Record{ID: fmt.Sprintf("n%d", i), Goal: fmt.Sprintf("goal %d", i), Status: task.Abstained})
	}
	_, abstained, _, examples := incompleteNodes(recs)
	if abstained != 10 {
		t.Fatalf("abstained = %d, want 10", abstained)
	}
	if len(examples) != 3 {
		t.Errorf("examples = %d, want capped at 3", len(examples))
	}
}

// TestTruncateGoalCapsLength: a long goal is truncated with an ellipsis rather than
// blowing up the clarify prompt.
func TestTruncateGoalCapsLength(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := truncateGoal(long)
	if len(got) > 80+len("…") { // 80 bytes of goal + the (multi-byte) ellipsis rune
		t.Errorf("truncateGoal length = %d, want capped at 80+len(ellipsis)", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncateGoal(%d chars) = %q, want an ellipsis suffix", len(long), got)
	}
	if got2 := truncateGoal("short goal"); got2 != "short goal" {
		t.Errorf("truncateGoal(short) = %q, want unchanged", got2)
	}
}

// TestPlanStoppedContextMentionsGoal: the stopped-context note names the original goal so
// the model's follow-up doesn't lose track of what was being investigated.
func TestPlanStoppedContextMentionsGoal(t *testing.T) {
	got := planStoppedContext(task.Record{Goal: "review the documentation"})
	if !strings.Contains(got, "review the documentation") {
		t.Errorf("planStoppedContext = %q, want it to mention the goal", got)
	}
	if !strings.Contains(got, "stopped") {
		t.Errorf("planStoppedContext = %q, want it to say the investigation stopped", got)
	}
}

// TestConfirmPlanIncompleteCleanPlanSkipsPrompt: a fully-done plan proceeds without
// calling RequestDecision at all (no gate interaction to resolve) — the happy path must
// never interrupt.
func TestConfirmPlanIncompleteCleanPlanSkipsPrompt(t *testing.T) {
	o := testOrchestrator()
	root := task.Record{ID: "root", Goal: "review docs"}
	recs := []task.Record{{ID: "root", Goal: "review docs", Status: task.Done}}

	proceed, note, err := o.confirmPlanIncomplete(context.Background(), root, recs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proceed || note != "" {
		t.Fatalf("confirmPlanIncomplete(clean) = (%v, %q), want (true, \"\")", proceed, note)
	}
}

// TestConfirmPlanIncompleteAnswerAppendsGroundingNote reproduces the witty-falcon fix
// directly: an incomplete plan asks, and on "answer" returns a note that tells the model
// not to claim the unexecuted steps happened.
func TestConfirmPlanIncompleteAnswerAppendsGroundingNote(t *testing.T) {
	o := testOrchestrator()
	root := task.Record{ID: "task-936", Goal: "create docs_to_retire.md"}
	recs := []task.Record{
		{ID: "task-936", Goal: "create docs_to_retire.md", Status: task.Proposed},
		{ID: "task-936-3-5-3", Goal: "check remaining .github files", Status: task.Abstained},
		{ID: "task-936-fail", Goal: "a failed leaf", Status: task.Failed},
	}

	type result struct {
		proceed bool
		note    string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		proceed, note, err := o.confirmPlanIncomplete(context.Background(), root, recs)
		done <- result{proceed, note, err}
	}()
	for !o.gate.deliver("answer") {
		time.Sleep(time.Millisecond)
	}
	r := <-done
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !r.proceed {
		t.Fatal("proceed = false, want true on \"answer\"")
	}
	if !strings.Contains(r.note, "1 failed, 1 abstained, 1 never ran") {
		t.Errorf("note = %q, want it to report the counts (1 failed, 1 abstained, 1 never ran)", r.note)
	}
	if !strings.Contains(r.note, "do not claim") {
		t.Errorf("note = %q, want an explicit anti-hallucination instruction", r.note)
	}
}

// TestConfirmPlanIncompleteStopReturnsNoNote: choosing to stop returns proceed=false and
// no grounding note — the caller (runPlanPhase/runDecomposition) is expected to use
// planStoppedContext instead.
func TestConfirmPlanIncompleteStopReturnsNoNote(t *testing.T) {
	o := testOrchestrator()
	root := task.Record{ID: "root", Goal: "goal"}
	recs := []task.Record{{ID: "a", Goal: "a", Status: task.Abstained}}

	type result struct {
		proceed bool
		note    string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		proceed, note, err := o.confirmPlanIncomplete(context.Background(), root, recs)
		done <- result{proceed, note, err}
	}()
	for !o.gate.deliver("stop") {
		time.Sleep(time.Millisecond)
	}
	r := <-done
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if r.proceed || r.note != "" {
		t.Fatalf("confirmPlanIncomplete(stop) = (%v, %q), want (false, \"\")", r.proceed, r.note)
	}
}

// TestConfirmPlanIncompleteCtxCanceled: an interrupt while the clarify decision is
// pending returns an error and proceed=false, mirroring RequestApproval/RequestDecision's
// own ctx.Done() contract.
func TestConfirmPlanIncompleteCtxCanceled(t *testing.T) {
	o := testOrchestrator()
	root := task.Record{ID: "root", Goal: "goal"}
	recs := []task.Record{{ID: "a", Goal: "a", Status: task.Abstained}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	proceed, note, err := o.confirmPlanIncomplete(ctx, root, recs)
	if err == nil {
		t.Fatal("expected an error on a canceled context")
	}
	if proceed || note != "" {
		t.Fatalf("confirmPlanIncomplete(canceled) = (%v, %q), want (false, \"\")", proceed, note)
	}
}
