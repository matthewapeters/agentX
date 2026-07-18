// Package wavefront's Scheduler is ADR 0012's round-free decomposition engine
// (amended 2026-07-17): it mirrors internal/runtime/scheduler.Scheduler's
// continuous-dispatch concurrency skeleton exactly — deliberately a separate type,
// not a generalization of the original, so a wavefront regression cannot touch the
// continuous engine's already-proven dispatch loop — with the merge step replaced
// by classify-and-converge instead of decompose-and-join.
//
// Design: docs/architecture/adr/0012-wavefront-grounded-decomposition-and-shared-blackboard.md
// (2026-07-17 amendment and same-day addendum).
// Behavior: docs/architecture/behavior/adr/0012_wavefront_scheduler.feature.md.
package wavefront

import (
	"context"
	"fmt"
	"strings"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
)

// Scheduler runs a task.Graph to completion via continuous dispatch over its
// injected collaborators.
type Scheduler struct {
	graph      *task.Graph
	rootGoal   string
	classifier Classifier
	executor   scheduler.Executor
	// chat/synthesisTemplate back the fallback synthesis call (ADR 0012 §6) — a
	// node with no self-match, once its deps are all resolved, gets exactly one of
	// these, never a second classify call.
	chat              Chat
	synthesisTemplate string
	// condense is optional (nil ⇒ TruncateFindings directly) — mirrors
	// capturingExec's degrade-gracefully posture for a nil summarizer.
	condense CondenseFunc
	slots    int
	maxDepth int
	observer scheduler.Observer // reused type; nil ⇒ silent
	// validator checks a proposed Tool's args against its tool's contract before
	// it is wired into the graph (classifyWithRetry). nil disables validation
	// entirely — every Tool is trusted as-is, the pre-validation behavior.
	validator ToolValidator

	dispatchOrder []string
	peak          int
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithObserver attaches a lifecycle observer (nil is allowed and means no-op).
// Reuses scheduler.Observer directly — its NodeDispatched/NodeDecomposed/
// NodeCompleted shape fits wavefront's lifecycle well enough (a node's children,
// however they were produced, are still "decomposed from" it) that a separate
// interface isn't warranted.
func WithObserver(obs scheduler.Observer) Option {
	return func(s *Scheduler) { s.observer = obs }
}

// WithCondenser attaches oversized-output summarization for command-Need results
// (ADR 0012 §6/§8). Omitted ⇒ TruncateFindings directly, same posture as a nil
// artifactReader/summarizer elsewhere in this codebase.
func WithCondenser(c CondenseFunc) Option {
	return func(s *Scheduler) { s.condense = c }
}

// WithValidator attaches tool-contract validation for resolved Tool proposals
// (classifyWithRetry). Omitted ⇒ every Tool is trusted as-is, the
// pre-validation behavior — this is what let calm-fjord-2's incomplete
// list_dir/tree calls reach the executor and be silently denied.
func WithValidator(v ToolValidator) Option {
	return func(s *Scheduler) { s.validator = v }
}

// New builds a Scheduler over g, which must already contain its single root node
// (the caller's responsibility, mirroring decompose.DrainPlan's construction of
// scheduler.Scheduler) — rootGoal is that root's Goal, captured once here so
// worker goroutines never need to read the graph themselves (§3). chat/
// synthesisTemplate back the fallback synthesis call and are required, not
// optional: a node with no self-match and no synthesis capability would sit in
// awaitingResolution forever, never re-dispatched, never caught by the stall
// guard — a hang, not a degrade. slots < 1 is clamped to 1.
func New(g *task.Graph, rootGoal string, classifier Classifier, chat Chat, synthesisTemplate string, ex scheduler.Executor, slots, maxDepth int, opts ...Option) *Scheduler {
	if slots < 1 {
		slots = 1
	}
	s := &Scheduler{
		graph: g, rootGoal: rootGoal, classifier: classifier,
		chat: chat, synthesisTemplate: synthesisTemplate,
		executor: ex, slots: slots, maxDepth: maxDepth,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// DispatchOrder returns the ids in the order the scheduler spawned work for them.
func (s *Scheduler) DispatchOrder() []string { return s.dispatchOrder }

// Peak is the maximum number of workers that were in flight at once (<= slots).
func (s *Scheduler) Peak() int { return s.peak }

type workKind int

const (
	doneClassify   workKind = iota // classify succeeded; merge step processes the Result
	doneExecute                    // a command-Need's execution finished
	doneSynthesize                 // a fallback synthesis call finished
	doneAsk                        // bounded-recursion clarify (KindStep at maxDepth)
	doneError                      // classify itself failed (a Go error, not a valid-but-negative Result)
)

type workDone struct {
	id      string
	kind    workKind
	result  Result      // for doneClassify
	status  task.Status // for doneExecute, doneSynthesize
	value   string      // for doneExecute, doneSynthesize (on Done)
	errText string      // for doneExecute, doneSynthesize (on Failed), and doneError
}

// Run drives the graph to a terminal state: every node is done, failed, marked for
// clarification, or permanently blocked by a failed dependency. Mirrors
// scheduler.Scheduler.Run's exact shape — depth/inflight/classified/awaiting are
// local, main-loop-only bookkeeping, never struct fields, so nothing outside this
// function can mutate them out of step with the dispatch loop.
func (s *Scheduler) Run(ctx context.Context) error {
	depth := map[string]int{}
	for _, n := range s.graph.Nodes() {
		depth[n.ID] = 0
	}
	inflight := map[string]bool{}
	classified := map[string]bool{}  // classified/executed at most once, ever — mirrors dispatched
	awaiting := map[string]bool{}    // waiting on spawned deps, needs a resolve-check — mirrors decomposed
	done := make(chan workDone, s.slots)
	active := 0

	for {
		for {
			progressed := false
			for _, id := range s.graph.Ready() {
				if inflight[id] {
					continue
				}
				if awaiting[id] {
					rec, ok := s.graph.Node(id)
					if !ok {
						continue
					}
					if match, ok := findExistingNode(s.graph, rec.Goal); ok && match.Status == task.Done && match.ID != id {
						s.setStatus(id, task.Done, match.Value, "")
						progressed = true
						continue
					}
					if active >= s.slots {
						continue
					}
					inflight[id] = true
					active++
					if active > s.peak {
						s.peak = active
					}
					go s.synthesize(ctx, rec, s.renderWM(), done)
					progressed = true
					continue
				}
				if active >= s.slots {
					continue
				}
				if classified[id] {
					return scheduler.ErrStalled // re-dispatch ⇒ a completion made no progress
				}
				rec, _ := s.graph.Node(id)
				inflight[id] = true
				classified[id] = true
				active++
				if active > s.peak {
					s.peak = active
				}
				s.dispatchOrder = append(s.dispatchOrder, id)
				if s.observer != nil {
					s.observer.NodeDispatched(rec, depth[id])
				}
				go s.work(ctx, rec, depth[id], s.renderWM(), done)
				progressed = true
			}
			if !progressed {
				break
			}
		}

		if active == 0 {
			return nil
		}

		var wd workDone
		select {
		case wd = <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
		active--
		delete(inflight, wd.id)
		switch wd.kind {
		case doneClassify:
			s.applyClassify(wd.id, depth, wd.result, awaiting)
		case doneExecute, doneSynthesize:
			s.setStatus(wd.id, wd.status, wd.value, wd.errText)
		case doneAsk:
			s.setStatus(wd.id, task.Abstained, "", "")
		case doneError:
			s.setStatus(wd.id, task.Failed, "", wd.errText)
		}
	}
}

// work runs one candidate off the main loop: dispatch on the node's declared Kind
// — a Task executes (a resolved command-Need), a Step classifies again (a further
// question). wm is a pre-rendered snapshot from the main loop (§3) — work must
// never read s.graph itself, since it runs on a worker goroutine.
func (s *Scheduler) work(ctx context.Context, rec task.Record, depth int, wm string, done chan<- workDone) {
	switch rec.Kind {
	case task.KindTask:
		status, value, errText := s.execute(ctx, rec)
		done <- workDone{id: rec.ID, kind: doneExecute, status: status, value: value, errText: errText}
	case task.KindStep:
		if depth >= s.maxDepth {
			done <- workDone{id: rec.ID, kind: doneAsk}
			return
		}
		result, err := s.classifyWithRetry(ctx, wm, rec.Goal)
		if err != nil {
			done <- workDone{id: rec.ID, kind: doneError, errText: fmt.Sprintf("classify failed: %v", err)}
			return
		}
		done <- workDone{id: rec.ID, kind: doneClassify, result: result}
	default:
		done <- workDone{id: rec.ID, kind: doneError, errText: fmt.Sprintf("node has no valid Kind (got %q)", rec.Kind)}
	}
}

// maxToolRetries bounds how many times classifyWithRetry re-asks the same
// question after an invalid TOOL proposal before giving up — a model that
// cannot self-correct in a couple of tries shouldn't stall the whole node.
const maxToolRetries = 2

// classifyWithRetry calls Classify, and — when s.validator rejects one or more
// proposed Tools' args — retries up to maxToolRetries times with the same wm
// and a question augmented by exactly what was wrong (RenderToolRetryFeedback).
// wm and the base question are never mutated or replaced: every retry sees its
// full original context plus, additively, what failed last time — never a
// narrower or summarized substitute for it. A nil s.validator disables this
// entirely (Classify's result passes straight through).
//
// Once retries are exhausted, any Tool still invalid is not silently dropped —
// that would reintroduce the calm-fjord-2 gap one layer up, just before the
// executor instead of inside it. It is folded into an equivalent open Need
// instead (demoteInvalidTools), so the information it was chasing stays live
// for a later round rather than vanishing without a trace.
func (s *Scheduler) classifyWithRetry(ctx context.Context, wm, question string) (Result, error) {
	if s.validator == nil {
		return s.classifier.Classify(ctx, wm, question)
	}
	q := question
	for attempt := 0; ; attempt++ {
		result, err := s.classifier.Classify(ctx, wm, q)
		if err != nil {
			return Result{}, err
		}
		bad := s.invalidTools(result.Tools)
		if len(bad) == 0 {
			return result, nil
		}
		if attempt >= maxToolRetries {
			return demoteInvalidTools(result, bad), nil
		}
		q = question + RenderToolRetryFeedback(bad)
	}
}

// invalidTools validates each of proposed against s.validator, returning a
// ToolFailure for every one that fails.
func (s *Scheduler) invalidTools(proposed []Tool) []ToolFailure {
	var bad []ToolFailure
	for _, t := range proposed {
		if err := s.validator.Validate(t.Command.Tool, t.Command.Args); err != nil {
			bad = append(bad, ToolFailure{
				Name: t.Name, Tool: t.Command.Tool, Err: err,
				Contract: s.validator.Describe(t.Command.Tool),
			})
		}
	}
	return bad
}

// demoteInvalidTools returns a copy of result with every Tool named in bad
// converted to an open Need instead — see classifyWithRetry's doc comment.
func demoteInvalidTools(result Result, bad []ToolFailure) Result {
	badNames := make(map[string]bool, len(bad))
	for _, f := range bad {
		badNames[f.Name] = true
	}
	out := Result{Knows: result.Knows, Needs: append([]Need(nil), result.Needs...)}
	for _, t := range result.Tools {
		if badNames[t.Name] {
			out.Needs = append(out.Needs, Need{Name: t.Name})
			continue
		}
		out.Tools = append(out.Tools, t)
	}
	return out
}

// execute drains a command-Need leaf through the executor and maps its outcome to
// a terminal status plus Value/Error, reusing the exact same mapping
// scheduler.Scheduler.execute uses (ADR 0012 §8) — Denied/NeedsApproval carry
// neither, scoped the same way and for the same reason (TOOL-7). Oversized results
// are condensed (§8) before being stored as Value.
func (s *Scheduler) execute(ctx context.Context, rec task.Record) (status task.Status, value, errText string) {
	out := s.executor.Execute(ctx, rec)
	switch out.Status {
	case executor.Executed:
		v := strings.TrimSpace(out.Result.Preview)
		if len(v) > OutputSummaryThreshold {
			v = s.condenseValue(ctx, rec.Goal, v)
		}
		return task.Done, v, ""
	case executor.Denied, executor.NeedsApproval:
		return task.Denied, "", ""
	default:
		return task.Failed, "", out.Reason
	}
}

// condenseValue applies s.condense if wired, else falls back to TruncateFindings
// directly. The chain is [rootGoal, goal] (collapsed to one item when the
// executing node's own goal already is the root's) — the same two-level
// simplification of totAlX's full ancestry walk that capturingExec.condense uses
// for the continuous engine (ADR 0012 §6), not the full chain from the executing
// node up through every intermediate question.
func (s *Scheduler) condenseValue(ctx context.Context, goal, text string) string {
	if s.condense == nil {
		return TruncateFindings(text)
	}
	chain := []string{s.rootGoal}
	if goal != s.rootGoal {
		chain = append(chain, goal)
	}
	return s.condense(ctx, chain, text)
}

// synthesize runs the fallback synthesis call (ADR 0012 §6) for a node whose deps
// are all resolved but has no self-match — the two-phase decompose-then-synthesize
// fix, dispatched only for nodes that need it. No "insufficient information"
// special-casing: the synthesis prompt already instructs the model to say so
// plainly when it can't answer, and that prose is a legitimate Value, not an error.
func (s *Scheduler) synthesize(ctx context.Context, rec task.Record, wm string, done chan<- workDone) {
	tmpl := s.synthesisTemplate
	if tmpl == "" {
		tmpl = DefaultSynthesisPromptTemplate
	}
	usr := RenderSynthesisUser(DefaultSynthesisUserTemplate, wm, rec.Goal)
	answer, err := s.chat(ctx, tmpl, usr, nil)
	if err != nil {
		done <- workDone{id: rec.ID, kind: doneSynthesize, status: task.Failed, errText: err.Error()}
		return
	}
	done <- workDone{id: rec.ID, kind: doneSynthesize, status: task.Done, value: strings.TrimSpace(answer)}
}

// setStatus rewrites a node's status, Value, and Error in place and notifies the
// observer — identical semantics to scheduler.Scheduler.setStatus (ADR 0012 §8).
// Every terminal transition, wherever it happens (the dispatch loop's inline
// convergence resolution included), must flow through here rather than notifying
// the observer at each call site separately — that duplication is exactly how a
// transition silently ends up unobserved.  Main-loop only.
func (s *Scheduler) setStatus(id string, st task.Status, value, errText string) {
	rec, ok := s.graph.Node(id)
	if !ok {
		return
	}
	rec.Status = st
	rec.Value = value
	rec.Error = errText
	_ = s.graph.Update(rec)
	if s.observer != nil {
		s.observer.NodeCompleted(id, st, value, errText)
	}
}
