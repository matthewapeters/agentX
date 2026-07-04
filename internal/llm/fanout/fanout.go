// Package fanout is the parallel model-invocation primitive for agentX: a
// bounded-concurrency pool that runs N model invocations, isolates their errors
// and timeouts, enforces each invocation's output contract, and folds the
// conforming results through a pluggable aggregator.
//
// It is the enabling capability for accuracy-first behaviors — self-consistency
// voting, tool-selection-by-commonality, parallel async jobs, parallel
// confirmations — which are all the same scatter-gather differing only in how
// invocations are generated and how results are folded. This package is the
// primitive; those behaviors are built on top.
//
// Behavior contract: tests/features/llm/fanout_pool.feature.
package fanout

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Params carries the per-invocation model knobs that make voting meaningful:
// varying temperature/seed across otherwise-identical invocations is what turns
// a fan-out into a diversity signal rather than N identical answers.
type Params struct {
	Temperature float64
	Seed        int
}

// Contract bounds an invocation's result so that fan-in stays comparable and the
// batch stays bounded. A result that violates the contract is quarantined out of
// the fold rather than failing the batch or poisoning the vote.
type Contract struct {
	// RequireField, when set, demands a non-empty Response.Fields[RequireField]
	// ("answer in this exact structure").
	RequireField string
	// MaxWords, when > 0, caps Response.Text length ("answer in 250 words").
	MaxWords int
	// MaxMilestones, when > 0, caps len(Response.Milestones) ("decompose into no
	// more than 5 milestones").
	MaxMilestones int
}

// check returns the quarantine reason for a response, or "" if it conforms.
func (c Contract) check(r Response) string {
	if c.RequireField != "" && strings.TrimSpace(r.Fields[c.RequireField]) == "" {
		return "malformed"
	}
	if c.MaxWords > 0 && len(strings.Fields(r.Text)) > c.MaxWords {
		return "over length"
	}
	if c.MaxMilestones > 0 && len(r.Milestones) > c.MaxMilestones {
		return "too many milestones"
	}
	return ""
}

// Invocation is one unit of fan-out work.
type Invocation struct {
	Tag      string // provenance handle, e.g. "classify-turn#2"
	Model    string
	Prompt   string
	Params   Params
	Timeout  time.Duration // per-invocation; 0 inherits the batch context deadline
	Contract Contract
}

// Response is a model's structured answer to one invocation. The structured
// fields (not just Text) are what the aggregators vote over and the contract
// validates.
type Response struct {
	Verdict    string            // the votable answer
	Confidence float64           // the model's self-reported confidence, if any
	Fields     map[string]string // structural payload, checked by Contract.RequireField
	Text       string            // raw prose, measured by Contract.MaxWords
	Milestones []string          // decomposition, counted by Contract.MaxMilestones
}

// Result is the outcome of a single invocation.
type Result struct {
	Inv        Invocation
	Response   Response
	Latency    time.Duration
	Err        error  // non-nil on model/timeout/cancellation failure
	Quarantine string // "" if conforming; else the contract violation reason
}

// Conforms reports whether a result may participate in a fold: it neither errored
// nor violated its output contract.
func (r Result) Conforms() bool { return r.Err == nil && r.Quarantine == "" }

// Invoker runs one invocation against a model backend. The production impl wraps
// the Ollama client; tests stub it for deterministic, backend-free scenarios.
type Invoker interface {
	Invoke(ctx context.Context, inv Invocation) (Response, error)
}

// InvokerFunc adapts a function to the Invoker interface.
type InvokerFunc func(ctx context.Context, inv Invocation) (Response, error)

// Invoke implements Invoker.
func (f InvokerFunc) Invoke(ctx context.Context, inv Invocation) (Response, error) {
	return f(ctx, inv)
}

// Recorder receives provenance for every invocation and the aggregate decision,
// so a fold is later answerable from the event log. Optional.
type Recorder interface {
	RecordInvocation(Result)
	RecordDecision(Decision)
}

// Pool fans invocations out under a concurrency cap and folds their results.
type Pool struct {
	invoker     Invoker
	concurrency int
	maxWidth    int
	recorder    Recorder
}

// Option configures a Pool.
type Option func(*Pool)

// WithConcurrency caps the number of invocations in flight at once.
func WithConcurrency(n int) Option {
	return func(p *Pool) {
		if n > 0 {
			p.concurrency = n
		}
	}
}

// WithMaxWidth caps how many invocations a single batch may contain. A wider
// batch is rejected — this bounds both cost and the blast radius of a runaway.
func WithMaxWidth(n int) Option { return func(p *Pool) { p.maxWidth = n } }

// WithRecorder attaches provenance recording.
func WithRecorder(r Recorder) Option { return func(p *Pool) { p.recorder = r } }

// New builds a Pool over the given invoker.
func New(inv Invoker, opts ...Option) *Pool {
	p := &Pool{invoker: inv, concurrency: 4}
	for _, o := range opts {
		o(p)
	}
	return p
}

// ErrWidthExceeded is returned when a batch is wider than the pool's budget.
var ErrWidthExceeded = errors.New("width exceeds budget")

func (p *Pool) checkWidth(n int) error {
	if p.maxWidth > 0 && n > p.maxWidth {
		return fmt.Errorf("%w: %d > %d", ErrWidthExceeded, n, p.maxWidth)
	}
	return nil
}

// invokeOne runs a single invocation with its own timeout and applies the output
// contract to a successful response.
func (p *Pool) invokeOne(ctx context.Context, inv Invocation) Result {
	start := time.Now()
	ictx := ctx
	if inv.Timeout > 0 {
		var cancel context.CancelFunc
		ictx, cancel = context.WithTimeout(ctx, inv.Timeout)
		defer cancel()
	}
	resp, err := p.invoker.Invoke(ictx, inv)
	r := Result{Inv: inv, Response: resp, Latency: time.Since(start), Err: err}
	if err == nil {
		r.Quarantine = inv.Contract.check(resp)
	}
	return r
}

// stream fans invs out under the concurrency cap, delivering each Result on the
// returned channel as it completes. The channel closes once every worker exits
// (including early, on ctx cancellation).
func (p *Pool) stream(ctx context.Context, invs []Invocation) <-chan Result {
	out := make(chan Result)
	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup

	for _, inv := range invs {
		wg.Add(1)
		go func(inv Invocation) {
			defer wg.Done()
			// Acquire a concurrency slot, or bail if the batch is torn down.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			r := p.invokeOne(ctx, inv)
			if p.recorder != nil {
				p.recorder.RecordInvocation(r)
			}
			select {
			case out <- r:
			case <-ctx.Done():
			}
		}(inv)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Run fans the batch out and collects every result (the collect-all fold). It
// rejects a batch wider than the pool budget before dispatching anything.
func (p *Pool) Run(ctx context.Context, invs []Invocation) ([]Result, error) {
	if err := p.checkWidth(len(invs)); err != nil {
		return nil, err
	}
	var out []Result
	for r := range p.stream(ctx, invs) {
		out = append(out, r)
	}
	return out, nil
}

// Fold fans the batch out and folds the conforming results through agg. It
// supports early exit: once the aggregator is Satisfied (its quorum is reached),
// the remaining in-flight invocations are cancelled and the decision returns
// without waiting for them.
func (p *Pool) Fold(ctx context.Context, invs []Invocation, agg Aggregator) (Decision, error) {
	if err := p.checkWidth(len(invs)); err != nil {
		return Decision{}, err
	}

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := p.stream(cctx, invs)
	for r := range ch {
		if !r.Conforms() {
			continue // quarantined or errored: never counted in the fold
		}
		agg.Add(r)
		if agg.Satisfied() {
			cancel() // early-exit: stop the stragglers
			break
		}
	}
	// Drain any results still in flight so the worker goroutines can exit.
	for range ch {
	}

	d := agg.Decide()
	if p.recorder != nil {
		p.recorder.RecordDecision(d)
	}
	return d, nil
}

// Decision is the outcome of a fold.
type Decision struct {
	Verdict    string
	Confidence float64
	Abstained  bool
	Reason     string         // set when Abstained
	Spread     map[string]int // vote spread, for provenance
}

// Aggregator folds streaming results into a Decision. Add incorporates a
// conforming result; Satisfied signals the pool may early-exit; Decide produces
// the final verdict from whatever has been added.
type Aggregator interface {
	Add(Result)
	Satisfied() bool
	Decide() Decision
}

// MajorityVote is the self-consistency aggregator: the modal verdict wins, with a
// confidence equal to its vote share. It early-exits at Quorum agreeing votes and
// abstains — rather than guessing — when the evidence is too thin or too scattered.
type MajorityVote struct {
	// Quorum, when > 0, is the agreeing-vote count that triggers early exit. It
	// also gates abstention: fewer conforming votes than Quorum abstains.
	Quorum int
	// AbstainBelow, when > 0, abstains if the winning share is below it.
	AbstainBelow float64

	counts map[string]int
	total  int
}

// VoteOption configures a MajorityVote.
type VoteOption func(*MajorityVote)

// WithQuorum sets the early-exit / minimum-evidence threshold.
func WithQuorum(n int) VoteOption { return func(m *MajorityVote) { m.Quorum = n } }

// WithAbstainBelow sets the winning-share floor below which the fold abstains.
func WithAbstainBelow(f float64) VoteOption { return func(m *MajorityVote) { m.AbstainBelow = f } }

// NewMajorityVote builds a majority-vote aggregator.
func NewMajorityVote(opts ...VoteOption) *MajorityVote {
	m := &MajorityVote{counts: map[string]int{}}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Add incorporates one conforming result's verdict.
func (m *MajorityVote) Add(r Result) {
	if m.counts == nil {
		m.counts = map[string]int{}
	}
	m.counts[r.Response.Verdict]++
	m.total++
}

// Satisfied reports whether any verdict has reached the quorum.
func (m *MajorityVote) Satisfied() bool {
	if m.Quorum <= 0 {
		return false
	}
	for _, c := range m.counts {
		if c >= m.Quorum {
			return true
		}
	}
	return false
}

// Decide resolves the vote, abstaining on insufficient or scattered evidence.
func (m *MajorityVote) Decide() Decision {
	spread := make(map[string]int, len(m.counts))
	for k, v := range m.counts {
		spread[k] = v
	}

	// Too few conforming votes to possibly reach quorum: abstain on thin evidence.
	if m.total == 0 || (m.Quorum > 0 && m.total < m.Quorum) {
		return Decision{Abstained: true, Reason: "insufficient conforming results", Spread: spread}
	}

	top, topN := "", 0
	for k, v := range m.counts {
		if v > topN {
			top, topN = k, v
		}
	}
	conf := float64(topN) / float64(m.total)

	// Enough votes, but no verdict commands a confident share: abstain, don't guess.
	if m.AbstainBelow > 0 && conf < m.AbstainBelow {
		return Decision{Abstained: true, Reason: "no quorum", Spread: spread}
	}
	return Decision{Verdict: top, Confidence: conf, Spread: spread}
}

// CollectAll is a trivial aggregator that decides nothing; it exists so callers
// can express "just run and give me the results" symmetrically with Run.
type CollectAll struct{ results []Result }

// Add records a result. Satisfied is never true (no early exit). Decide is empty.
func (c *CollectAll) Add(r Result)      { c.results = append(c.results, r) }
func (c *CollectAll) Satisfied() bool   { return false }
func (c *CollectAll) Decide() Decision  { return Decision{} }
func (c *CollectAll) Results() []Result { return c.results }
