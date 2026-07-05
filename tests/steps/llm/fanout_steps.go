// Package llmsteps implements the Godog steps for the LLM behavior domain.
// Currently: the parallel model-invocation pool (tests/features/llm/fanout_pool.feature).
// The model backend is stubbed so scenarios are deterministic and Ollama-free.
package llmsteps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
)

// behavior scripts how the stub invoker answers one invocation, keyed by tag.
type behavior struct {
	verdict    string
	failMsg    string
	block      bool // block until the release channel closes (or ctx cancels)
	delay      time.Duration
	fields     map[string]string
	text       string
	milestones []string
}

// captureRecorder records provenance for assertion.
type captureRecorder struct {
	mu       sync.Mutex
	invs     []fanout.Result
	decided  bool
	decision fanout.Decision
}

func (c *captureRecorder) RecordInvocation(r fanout.Result) {
	c.mu.Lock()
	c.invs = append(c.invs, r)
	c.mu.Unlock()
}

func (c *captureRecorder) RecordDecision(d fanout.Decision) {
	c.mu.Lock()
	c.decided = true
	c.decision = d
	c.mu.Unlock()
}

type fanoutWorld struct {
	pool        *fanout.Pool
	invoker     fanout.Invoker
	behaviors   map[string]*behavior
	invs        []fanout.Invocation
	agg         *fanout.MajorityVote
	contract    fanout.Contract
	rec         *captureRecorder
	concurrency int

	results  []fanout.Result
	decision fanout.Decision
	runErr   error

	// concurrency + lifecycle instrumentation
	mu           sync.Mutex
	inflight     int
	maxFlight    int
	seenTemps    []float64
	cancelled    map[string]bool
	release      chan struct{}
	entered      chan string
	blockingCount int
	slowTags     []string

	// async run bookkeeping (cancellation scenario)
	cancel       context.CancelFunc
	done         chan struct{}
	cancelAt     time.Time
	cancelElapsed time.Duration

	// env restore for the server-defaults scenarios
	envRestore func()
}

// InitializeScenario registers the LLM-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &fanoutWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.envRestore != nil {
			w.envRestore()
		}
		*w = fanoutWorld{}
		return ctx, err
	})

	// --- pool construction ---
	sc.Step(`^a fan-out pool backed by a stub model$`, w.poolStub)
	sc.Step(`^a fan-out pool with a concurrency cap of (\d+)$`, w.poolCap)
	sc.Step(`^a fan-out pool with a maximum width of (\d+)$`, w.poolWidth)
	sc.Step(`^a fan-out pool with provenance recording enabled$`, w.poolRecorder)
	sc.Step(`^a fan-out pool backed by a stub model with invocations in flight$`, w.poolInFlight)
	sc.Step(`^the Ollama parallel-slot count is "([^"]*)"$`, w.slotsEnv)
	sc.Step(`^the Ollama parallel-slot count is unset$`, w.slotsUnset)
	sc.Step(`^a pool is built with server defaults$`, w.poolServerDefaults)

	// --- aggregators ---
	sc.Step(`^a majority-vote aggregator$`, w.majAgg)
	sc.Step(`^a majority-vote aggregator with a quorum of (\d+)$`, w.majAggQuorum)
	sc.Step(`^a majority-vote aggregator with an abstain threshold of ([\d.]+)$`, w.majAggAbstain)

	// --- invocation batches ---
	sc.Step(`^a batch of (\d+) invocations each tagged with its purpose$`, w.batchTagged)
	sc.Step(`^3 invocations with temperatures 0\.0, 0\.5, and 1\.0$`, w.batchTemps)
	sc.Step(`^(\d+) invocations that each block until released$`, w.batchBlocking)
	sc.Step(`^a batch of 3 invocations where the second fails with "([^"]*)"$`, w.batchSecondFails)
	sc.Step(`^an invocation with a 50ms timeout whose model answers after 200ms$`, w.invSlowTimeout)
	sc.Step(`^a sibling invocation that answers promptly$`, w.invPrompt)
	sc.Step(`^fan-out invocations returning verdicts "([^"]*)"$`, w.invVerdicts)
	sc.Step(`^5 invocations where 3 quickly agree on "([^"]*)" and 2 are slow$`, w.batchQuorumSlow)
	sc.Step(`^a batch of (\d+) invocations folded by majority vote$`, w.batchForVote)

	// --- output contracts ---
	sc.Step(`^an output contract requiring a "([^"]*)" field$`, w.contractField)
	sc.Step(`^an output contract bounding the answer to (\d+) words$`, w.contractWords)
	sc.Step(`^an output contract allowing at most (\d+) milestones$`, w.contractMilestones)
	sc.Step(`^(\d+) fan-out results where one omits the "([^"]*)" field$`, w.resultsOneOmits)
	sc.Step(`^a fan-out result of (\d+) words$`, w.resultWords)
	sc.Step(`^a fan-out result decomposing the goal into (\d+) milestones$`, w.resultMilestones)
	sc.Step(`^(\d+) fan-out results where only (\d+) conform to the output contract$`, w.resultsOnlyNConform)

	// --- actions ---
	sc.Step(`^the pool runs the batch with a collect-all aggregator$`, w.runCollect)
	sc.Step(`^the pool runs the batch$`, w.runsBatch)
	sc.Step(`^the pool folds the results$`, w.foldsResults)
	sc.Step(`^the pool validates the results? against the contract$`, w.validates)
	sc.Step(`^a batch of 12 invocations is submitted$`, w.submitTwelve)
	sc.Step(`^the caller cancels the fan-out context$`, w.cancelsCtx)

	// --- assertions ---
	sc.Step(`^the fan-out returns (\d+) results$`, w.returnsN)
	sc.Step(`^each fan-out result carries its invocation tag$`, w.eachCarriesTag)
	sc.Step(`^each invocation is dispatched with its own temperature$`, w.dispatchedTemps)
	sc.Step(`^no more than (\d+) invocations are in flight at any moment$`, w.maxInFlight)
	sc.Step(`^(\d+) fan-out results? succeed$`, w.nSucceed)
	sc.Step(`^(\d+) fan-out result carries the error "([^"]*)"$`, w.nCarryError)
	sc.Step(`^the fan-out batch itself does not error$`, w.batchNoError)
	sc.Step(`^the slow fan-out result carries a timeout error$`, w.slowTimedOut)
	sc.Step(`^the sibling fan-out result succeeds$`, w.siblingSucceeds)
	sc.Step(`^the in-flight invocations are cancelled$`, w.inflightCancelled)
	sc.Step(`^the pool returns without waiting for the full timeout$`, w.returnedPromptly)
	sc.Step(`^the fold decision is "([^"]*)"$`, w.foldDecision)
	sc.Step(`^the fold confidence is ([\d.]+)$`, w.foldConfidence)
	sc.Step(`^the (\d+) slow invocations are cancelled$`, w.slowCancelled)
	sc.Step(`^the fold abstains$`, w.foldAbstains)
	sc.Step(`^the abstention reason is "([^"]*)"$`, w.abstainReason)
	sc.Step(`^(\d+) fan-out results? conform$`, w.nConform)
	sc.Step(`^(\d+) fan-out results? (?:is|are) quarantined as "([^"]*)"$`, w.nQuarantined)
	sc.Step(`^the fan-out result is quarantined as "([^"]*)"$`, w.oneQuarantined)
	sc.Step(`^only conforming results are counted in the fold$`, w.onlyConformingCounted)
	sc.Step(`^the fan-out is rejected with reason "([^"]*)"$`, w.rejectedWith)
	sc.Step(`^each invocation is recorded as an event$`, w.eachRecorded)
	sc.Step(`^the aggregate decision is recorded with its vote spread$`, w.decisionRecorded)
	sc.Step(`^the pool default concurrency is (\d+)$`, w.defaultConcurrency)
	sc.Step(`^the pool default width budget is (\d+)$`, w.defaultWidth)
}

// ---- server-defaults steps ----

func (w *fanoutWorld) captureSlotsEnv() {
	old, had := os.LookupEnv("OLLAMA_NUM_PARALLEL")
	w.envRestore = func() {
		if had {
			_ = os.Setenv("OLLAMA_NUM_PARALLEL", old)
		} else {
			_ = os.Unsetenv("OLLAMA_NUM_PARALLEL")
		}
	}
}

func (w *fanoutWorld) slotsEnv(val string) error {
	w.captureSlotsEnv()
	return os.Setenv("OLLAMA_NUM_PARALLEL", val)
}

func (w *fanoutWorld) slotsUnset() error {
	w.captureSlotsEnv()
	return os.Unsetenv("OLLAMA_NUM_PARALLEL")
}

func (w *fanoutWorld) poolServerDefaults() error {
	w.ensureInvoker()
	w.pool = fanout.New(w.invoker, fanout.WithServerDefaults())
	return nil
}

func (w *fanoutWorld) defaultConcurrency(n int) error {
	if got := w.pool.Concurrency(); got != n {
		return fmt.Errorf("pool concurrency = %d, want %d", got, n)
	}
	return nil
}

func (w *fanoutWorld) defaultWidth(n int) error {
	if got := w.pool.MaxWidth(); got != n {
		return fmt.Errorf("pool width budget = %d, want %d", got, n)
	}
	return nil
}

// ---- stub invoker ----

func (w *fanoutWorld) ensureInvoker() {
	if w.invoker != nil {
		return
	}
	w.behaviors = map[string]*behavior{}
	w.cancelled = map[string]bool{}
	w.release = make(chan struct{})
	w.entered = make(chan string, 128)
	if w.concurrency == 0 {
		w.concurrency = 8
	}
	w.invoker = fanout.InvokerFunc(w.invoke)
}

func (w *fanoutWorld) ensurePool() {
	w.ensureInvoker()
	if w.pool == nil {
		w.pool = fanout.New(w.invoker, fanout.WithConcurrency(w.concurrency))
	}
}

func (w *fanoutWorld) invoke(ctx context.Context, inv fanout.Invocation) (fanout.Response, error) {
	w.mu.Lock()
	w.seenTemps = append(w.seenTemps, inv.Params.Temperature)
	w.inflight++
	if w.inflight > w.maxFlight {
		w.maxFlight = w.inflight
	}
	b := w.behaviors[inv.Tag]
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.inflight--
		w.mu.Unlock()
	}()

	if b == nil {
		b = &behavior{}
	}
	if b.failMsg != "" {
		return fanout.Response{}, errors.New(b.failMsg)
	}
	if b.block {
		w.entered <- inv.Tag
		select {
		case <-w.release:
		case <-ctx.Done():
			w.markCancelled(inv.Tag)
			return fanout.Response{}, ctx.Err()
		}
	}
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			w.markCancelled(inv.Tag)
			return fanout.Response{}, ctx.Err()
		}
	}
	return fanout.Response{
		Verdict:    b.verdict,
		Fields:     b.fields,
		Text:       b.text,
		Milestones: b.milestones,
	}, nil
}

func (w *fanoutWorld) markCancelled(tag string) {
	w.mu.Lock()
	w.cancelled[tag] = true
	w.mu.Unlock()
}

func (w *fanoutWorld) cancelledN() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.cancelled)
}

func (w *fanoutWorld) isCancelled(tag string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cancelled[tag]
}

// ---- pool construction steps ----

func (w *fanoutWorld) poolStub() error {
	w.ensureInvoker()
	return nil
}

func (w *fanoutWorld) poolCap(n int) error {
	w.concurrency = n
	w.ensureInvoker()
	w.pool = fanout.New(w.invoker, fanout.WithConcurrency(n))
	return nil
}

func (w *fanoutWorld) poolWidth(n int) error {
	w.ensureInvoker()
	w.pool = fanout.New(w.invoker, fanout.WithConcurrency(w.concurrency), fanout.WithMaxWidth(n))
	return nil
}

func (w *fanoutWorld) poolRecorder() error {
	w.ensureInvoker()
	w.rec = &captureRecorder{}
	w.pool = fanout.New(w.invoker, fanout.WithConcurrency(w.concurrency), fanout.WithRecorder(w.rec))
	return nil
}

func (w *fanoutWorld) poolInFlight() error {
	w.ensureInvoker()
	w.pool = fanout.New(w.invoker, fanout.WithConcurrency(w.concurrency))
	for i := 0; i < 3; i++ {
		tag := fmt.Sprintf("blk%d", i)
		w.behaviors[tag] = &behavior{block: true, verdict: "ok"}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	w.blockingCount = 3

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		w.results, w.runErr = w.pool.Run(ctx, w.invs)
		close(w.done)
	}()

	want := w.concurrency
	if want > 3 {
		want = 3
	}
	for i := 0; i < want; i++ {
		select {
		case <-w.entered:
		case <-time.After(2 * time.Second):
			return fmt.Errorf("only %d/%d invocations reached flight", i, want)
		}
	}
	return nil
}

// ---- aggregator steps ----

func (w *fanoutWorld) majAgg() error {
	w.agg = fanout.NewMajorityVote()
	return nil
}

func (w *fanoutWorld) majAggQuorum(n int) error {
	w.agg = fanout.NewMajorityVote(fanout.WithQuorum(n))
	return nil
}

func (w *fanoutWorld) majAggAbstain(f float64) error {
	w.agg = fanout.NewMajorityVote(fanout.WithAbstainBelow(f))
	return nil
}

// ---- invocation batch steps ----

func (w *fanoutWorld) batchTagged(n int) error {
	w.ensureInvoker()
	for i := 0; i < n; i++ {
		tag := fmt.Sprintf("purpose-%d", i)
		w.behaviors[tag] = &behavior{verdict: "ok", fields: map[string]string{"verdict": "ok"}}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	return nil
}

func (w *fanoutWorld) batchTemps() error {
	w.ensureInvoker()
	for i, tp := range []float64{0.0, 0.5, 1.0} {
		tag := fmt.Sprintf("t%d", i)
		w.behaviors[tag] = &behavior{verdict: "ok"}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag, Params: fanout.Params{Temperature: tp}})
	}
	return nil
}

func (w *fanoutWorld) batchBlocking(n int) error {
	w.ensureInvoker()
	for i := 0; i < n; i++ {
		tag := fmt.Sprintf("blk%d", i)
		w.behaviors[tag] = &behavior{block: true, verdict: "ok"}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	w.blockingCount = n
	return nil
}

func (w *fanoutWorld) batchSecondFails(msg string) error {
	w.ensureInvoker()
	for i := 0; i < 3; i++ {
		tag := fmt.Sprintf("f%d", i)
		b := &behavior{verdict: "ok"}
		if i == 1 {
			b.failMsg = msg
		}
		w.behaviors[tag] = b
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	return nil
}

func (w *fanoutWorld) invSlowTimeout() error {
	w.ensureInvoker()
	w.behaviors["slow"] = &behavior{verdict: "ok", delay: 200 * time.Millisecond}
	w.invs = append(w.invs, fanout.Invocation{Tag: "slow", Timeout: 50 * time.Millisecond})
	return nil
}

func (w *fanoutWorld) invPrompt() error {
	w.ensureInvoker()
	w.behaviors["fast"] = &behavior{verdict: "ok"}
	w.invs = append(w.invs, fanout.Invocation{Tag: "fast"})
	return nil
}

func (w *fanoutWorld) invVerdicts(list string) error {
	w.ensureInvoker()
	for i, v := range strings.Split(list, ",") {
		v = strings.TrimSpace(v)
		tag := fmt.Sprintf("v%d", i)
		w.behaviors[tag] = &behavior{verdict: v, fields: map[string]string{"verdict": v}}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	return nil
}

func (w *fanoutWorld) batchQuorumSlow(verdict string) error {
	w.ensureInvoker()
	// The "fast" trio carries a small delay so that all five goroutines are
	// parked in their delay-select before the quorum is reached — otherwise a
	// straggler can be cut at the semaphore before it observes the cancel, which
	// makes "was it cancelled?" scheduling-dependent.
	for i := 0; i < 3; i++ {
		tag := fmt.Sprintf("fast%d", i)
		w.behaviors[tag] = &behavior{verdict: verdict, delay: 30 * time.Millisecond}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	for i := 0; i < 2; i++ {
		tag := fmt.Sprintf("slow%d", i)
		w.behaviors[tag] = &behavior{verdict: verdict, delay: 800 * time.Millisecond}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
		w.slowTags = append(w.slowTags, tag)
	}
	return nil
}

func (w *fanoutWorld) batchForVote(n int) error {
	w.ensureInvoker()
	for i := 0; i < n; i++ {
		tag := fmt.Sprintf("p%d", i)
		w.behaviors[tag] = &behavior{verdict: "write", fields: map[string]string{"verdict": "write"}}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag})
	}
	w.agg = fanout.NewMajorityVote()
	return nil
}

// ---- output-contract steps ----

func (w *fanoutWorld) contractField(field string) error {
	w.contract = fanout.Contract{RequireFields: []string{field}}
	return nil
}

func (w *fanoutWorld) contractWords(n int) error {
	w.contract = fanout.Contract{MaxWords: n}
	return nil
}

func (w *fanoutWorld) contractMilestones(n int) error {
	w.contract = fanout.Contract{MaxMilestones: n}
	return nil
}

func (w *fanoutWorld) resultsOneOmits(n int, field string) error {
	w.ensureInvoker()
	for i := 0; i < n; i++ {
		tag := fmt.Sprintf("c%d", i)
		fields := map[string]string{field: "write"}
		if i == n-1 {
			fields = map[string]string{} // the one that omits the field
		}
		w.behaviors[tag] = &behavior{verdict: "write", fields: fields}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag, Contract: w.contract})
	}
	return nil
}

func (w *fanoutWorld) resultWords(n int) error {
	w.ensureInvoker()
	text := strings.TrimSpace(strings.Repeat("word ", n))
	w.behaviors["long"] = &behavior{verdict: "ok", text: text}
	w.invs = append(w.invs, fanout.Invocation{Tag: "long", Contract: w.contract})
	return nil
}

func (w *fanoutWorld) resultMilestones(n int) error {
	w.ensureInvoker()
	ms := make([]string, n)
	for i := range ms {
		ms[i] = fmt.Sprintf("m%d", i)
	}
	w.behaviors["plan"] = &behavior{verdict: "ok", milestones: ms}
	w.invs = append(w.invs, fanout.Invocation{Tag: "plan", Contract: w.contract})
	return nil
}

func (w *fanoutWorld) resultsOnlyNConform(total, conform int) error {
	w.ensureInvoker()
	c := fanout.Contract{RequireFields: []string{"verdict"}}
	for i := 0; i < total; i++ {
		tag := fmt.Sprintf("k%d", i)
		fields := map[string]string{}
		if i < conform {
			fields = map[string]string{"verdict": "write"}
		}
		w.behaviors[tag] = &behavior{verdict: "write", fields: fields}
		w.invs = append(w.invs, fanout.Invocation{Tag: tag, Contract: c})
	}
	return nil
}

// ---- action steps ----

func (w *fanoutWorld) runCollect() error {
	w.ensurePool()
	w.results, w.runErr = w.pool.Run(context.Background(), w.invs)
	return nil
}

func (w *fanoutWorld) runsBatch() error {
	w.ensurePool()
	done := make(chan struct{})
	go func() {
		w.results, w.runErr = w.pool.Run(context.Background(), w.invs)
		close(done)
	}()

	if w.blockingCount > 0 {
		want := w.concurrency
		if w.blockingCount < want {
			want = w.blockingCount
		}
		for i := 0; i < want; i++ {
			select {
			case <-w.entered:
			case <-time.After(2 * time.Second):
				return fmt.Errorf("only %d/%d invocations entered flight", i, want)
			}
		}
		close(w.release)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("run did not complete")
	}
	return nil
}

func (w *fanoutWorld) foldsResults() error {
	w.ensurePool()
	if w.agg == nil {
		w.agg = fanout.NewMajorityVote()
	}
	w.decision, w.runErr = w.pool.Fold(context.Background(), w.invs, w.agg)
	return nil
}

func (w *fanoutWorld) validates() error {
	w.ensurePool()
	w.results, w.runErr = w.pool.Run(context.Background(), w.invs)
	return nil
}

func (w *fanoutWorld) submitTwelve() error {
	w.ensurePool()
	var invs []fanout.Invocation
	for i := 0; i < 12; i++ {
		invs = append(invs, fanout.Invocation{Tag: fmt.Sprintf("x%d", i)})
	}
	w.results, w.runErr = w.pool.Run(context.Background(), invs)
	return nil
}

func (w *fanoutWorld) cancelsCtx() error {
	w.cancelAt = time.Now()
	w.cancel()
	return nil
}

// ---- assertion steps ----

func (w *fanoutWorld) returnsN(n int) error {
	if len(w.results) != n {
		return fmt.Errorf("returned %d results, want %d", len(w.results), n)
	}
	return nil
}

func (w *fanoutWorld) eachCarriesTag() error {
	seen := map[string]bool{}
	for _, r := range w.results {
		if r.Inv.Tag == "" {
			return fmt.Errorf("a result carries an empty invocation tag")
		}
		seen[r.Inv.Tag] = true
	}
	if len(seen) != len(w.results) {
		return fmt.Errorf("tags not unique: %d tags for %d results", len(seen), len(w.results))
	}
	return nil
}

func (w *fanoutWorld) dispatchedTemps() error {
	want := []float64{0.0, 0.5, 1.0}
	for _, t := range want {
		found := false
		for _, s := range w.seenTemps {
			if math.Abs(s-t) < 1e-9 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("temperature %.1f was not dispatched (saw %v)", t, w.seenTemps)
		}
	}
	return nil
}

func (w *fanoutWorld) maxInFlight(n int) error {
	w.mu.Lock()
	got := w.maxFlight
	w.mu.Unlock()
	if got > n {
		return fmt.Errorf("max in flight %d exceeded cap %d", got, n)
	}
	if got != n {
		return fmt.Errorf("max in flight %d did not reach the cap %d (no real concurrency)", got, n)
	}
	return nil
}

func (w *fanoutWorld) nSucceed(n int) error {
	got := 0
	for _, r := range w.results {
		if r.Err == nil {
			got++
		}
	}
	if got != n {
		return fmt.Errorf("%d results succeeded, want %d", got, n)
	}
	return nil
}

func (w *fanoutWorld) nCarryError(n int, msg string) error {
	got := 0
	for _, r := range w.results {
		if r.Err != nil && r.Err.Error() == msg {
			got++
		}
	}
	if got != n {
		return fmt.Errorf("%d results carry error %q, want %d", got, msg, n)
	}
	return nil
}

func (w *fanoutWorld) batchNoError() error {
	if w.runErr != nil {
		return fmt.Errorf("batch errored: %v", w.runErr)
	}
	return nil
}

func (w *fanoutWorld) resultByTagPrefix(prefix string) (fanout.Result, bool) {
	for _, r := range w.results {
		if strings.HasPrefix(r.Inv.Tag, prefix) {
			return r, true
		}
	}
	return fanout.Result{}, false
}

func (w *fanoutWorld) slowTimedOut() error {
	r, ok := w.resultByTagPrefix("slow")
	if !ok {
		return fmt.Errorf("no slow result found")
	}
	if !errors.Is(r.Err, context.DeadlineExceeded) {
		return fmt.Errorf("slow result error = %v, want deadline exceeded", r.Err)
	}
	return nil
}

func (w *fanoutWorld) siblingSucceeds() error {
	r, ok := w.resultByTagPrefix("fast")
	if !ok {
		return fmt.Errorf("no sibling result found")
	}
	if r.Err != nil {
		return fmt.Errorf("sibling errored: %v", r.Err)
	}
	return nil
}

func (w *fanoutWorld) inflightCancelled() error {
	select {
	case <-w.done:
		w.cancelElapsed = time.Since(w.cancelAt)
	case <-time.After(2 * time.Second):
		return fmt.Errorf("pool did not return after cancellation")
	}
	if w.cancelledN() == 0 {
		return fmt.Errorf("no invocations were recorded as cancelled")
	}
	return nil
}

func (w *fanoutWorld) returnedPromptly() error {
	if w.cancelElapsed >= time.Second {
		return fmt.Errorf("pool took %v to return after cancel", w.cancelElapsed)
	}
	return nil
}

func (w *fanoutWorld) foldDecision(want string) error {
	if w.decision.Abstained {
		return fmt.Errorf("fold abstained (%s), want decision %q", w.decision.Reason, want)
	}
	if w.decision.Verdict != want {
		return fmt.Errorf("fold decision = %q, want %q", w.decision.Verdict, want)
	}
	return nil
}

func (w *fanoutWorld) foldConfidence(f float64) error {
	if math.Abs(w.decision.Confidence-f) > 1e-6 {
		return fmt.Errorf("fold confidence = %v, want %v", w.decision.Confidence, f)
	}
	return nil
}

func (w *fanoutWorld) slowCancelled(n int) error {
	got := 0
	for _, tag := range w.slowTags {
		if w.isCancelled(tag) {
			got++
		}
	}
	if got != n {
		return fmt.Errorf("%d slow invocations cancelled, want %d", got, n)
	}
	return nil
}

func (w *fanoutWorld) foldAbstains() error {
	if !w.decision.Abstained {
		return fmt.Errorf("fold did not abstain (decided %q)", w.decision.Verdict)
	}
	return nil
}

func (w *fanoutWorld) abstainReason(want string) error {
	if w.decision.Reason != want {
		return fmt.Errorf("abstention reason = %q, want %q", w.decision.Reason, want)
	}
	return nil
}

func (w *fanoutWorld) nConform(n int) error {
	got := 0
	for _, r := range w.results {
		if r.Conforms() {
			got++
		}
	}
	if got != n {
		return fmt.Errorf("%d results conform, want %d", got, n)
	}
	return nil
}

func (w *fanoutWorld) nQuarantined(n int, reason string) error {
	got := 0
	for _, r := range w.results {
		if r.Quarantine == reason {
			got++
		}
	}
	if got != n {
		return fmt.Errorf("%d results quarantined as %q, want %d", got, reason, n)
	}
	return nil
}

func (w *fanoutWorld) oneQuarantined(reason string) error {
	for _, r := range w.results {
		if r.Quarantine == reason {
			return nil
		}
	}
	return fmt.Errorf("no result quarantined as %q", reason)
}

func (w *fanoutWorld) onlyConformingCounted() error {
	agg := fanout.NewMajorityVote()
	d, err := w.pool.Fold(context.Background(), w.invs, agg)
	if err != nil {
		return err
	}
	total := 0
	for _, v := range d.Spread {
		total += v
	}
	if total != 3 {
		return fmt.Errorf("fold counted %d conforming results, want 3", total)
	}
	return nil
}

func (w *fanoutWorld) rejectedWith(reason string) error {
	if w.runErr == nil {
		return fmt.Errorf("batch was not rejected")
	}
	if !strings.Contains(w.runErr.Error(), reason) {
		return fmt.Errorf("rejection %q does not mention %q", w.runErr.Error(), reason)
	}
	return nil
}

func (w *fanoutWorld) eachRecorded() error {
	if w.rec == nil {
		return fmt.Errorf("no recorder attached")
	}
	if len(w.rec.invs) != len(w.invs) {
		return fmt.Errorf("recorded %d invocations, want %d", len(w.rec.invs), len(w.invs))
	}
	return nil
}

func (w *fanoutWorld) decisionRecorded() error {
	if w.rec == nil || !w.rec.decided {
		return fmt.Errorf("no decision was recorded")
	}
	if len(w.rec.decision.Spread) == 0 {
		return fmt.Errorf("recorded decision has no vote spread")
	}
	return nil
}
