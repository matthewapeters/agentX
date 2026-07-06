// Command ollamabench is a 1-vs-N concurrency probe for a local Ollama server.
//
// It answers two questions the app's design depends on:
//
//  1. Does the server actually run prompts concurrently, or does it serialize
//     them? (If serial, a round of N identical prompts takes ~N× a single
//     prompt; if concurrent, it takes ~1× plus contention.)
//  2. How does per-request latency and throughput degrade as N grows?
//
// For each concurrency level R in [min..max] it fires the SAME prompt at the
// SAME endpoint R times, released simultaneously via a start barrier so the
// fan-out is genuinely concurrent and not drip-fed. One result record per round
// is printed to stdout as soon as the round completes (tee it to a file).
//
// Concurrency is not assumed — it is measured:
//   - a closed-channel start barrier releases all R goroutines at once;
//   - an atomic in-flight gauge records the PEAK number of simultaneously
//     open requests (peak == R proves every request overlapped);
//   - concRatio = sum(per-request latency) / round wall time. ~1.0 means the
//     server serialized the round; ~R means it ran them in parallel.
//
// The prompt is read from a file (-prompt <path>) so large or multi-line
// prompts stay out of the shell and the exact bytes are reproducible.
//
// Usage:
//
//	go run ./cmd/ollamabench -max 8 -prompt ./prompt.txt | tee bench.log
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentx/internal/llm/ollama"
)

func main() {
	var (
		host    = flag.String("host", "localhost:11434", "Ollama host:port")
		model   = flag.String("model", "nemotron-cascade-2:latest", "model name (must be installed)")
		promptF = flag.String("prompt", "", "path to a file whose contents are the prompt sent to every request (required)")
		minR    = flag.Int("min", 1, "lowest concurrency level to test")
		maxR    = flag.Int("max", 8, "highest concurrency level to test (the N in 1-vs-N)")
		step    = flag.Int("step", 1, "increment between concurrency levels")
		timeout = flag.Duration("timeout", 120*time.Second, "per-request timeout")
		settle  = flag.Duration("settle", 500*time.Millisecond, "pause between rounds to let the server quiesce")
		numCtx  = flag.Int("numctx", 0, "options.num_ctx per request (0 = server default)")
		think   = flag.Bool("think", false, "ask the model to emit reasoning (counts toward TTFT/tokens)")
		warmup  = flag.Bool("warmup", true, "fire one throwaway request first to load the model")
	)
	flag.Parse()

	if *minR < 1 || *maxR < *minR || *step < 1 {
		fmt.Fprintln(os.Stderr, "invalid range: need 1 <= min <= max and step >= 1")
		os.Exit(2)
	}
	if *promptF == "" {
		fmt.Fprintln(os.Stderr, "the -prompt flag (path to a prompt file) is required")
		os.Exit(2)
	}
	promptBytes, err := os.ReadFile(*promptF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read prompt file: %v\n", err)
		os.Exit(2)
	}
	prompt := strings.TrimRight(string(promptBytes), "\r\n")
	if prompt == "" {
		fmt.Fprintf(os.Stderr, "prompt file %q is empty\n", *promptF)
		os.Exit(2)
	}

	// Tuned client: raise the per-host connection ceilings well above the
	// highest concurrency level so Go's connection pool never caps the fan-out.
	// A serialized round must then be the server's doing, not the client's.
	maxConns := *maxR + 4
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = maxConns
	transport.MaxIdleConns = maxConns
	transport.MaxIdleConnsPerHost = maxConns
	client := ollama.NewWithHTTPClient(*host, &http.Client{Transport: transport})

	// Readiness gate: fail fast with a clear message instead of N obscure
	// per-request errors if the host is down or the model is missing.
	readyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := client.Ready(readyCtx, *model); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "ollama not ready: %v\n", err)
		os.Exit(1)
	}
	cancel()

	fmt.Fprintf(os.Stderr, "# host=%s model=%s prompt=%s (%d bytes)\n", *host, *model, *promptF, len(prompt))
	fmt.Fprintf(os.Stderr, "# levels=%d..%d step=%d timeout=%s think=%v numctx=%d maxconns=%d\n",
		*minR, *maxR, *step, *timeout, *think, *numCtx, maxConns)

	// Warm the model so R=1 isn't skewed by a cold weight load.
	if *warmup {
		fmt.Fprintln(os.Stderr, "# warming up model...")
		r := runRound(client, *model, prompt, 1, *timeout, *numCtx, *think)
		fmt.Fprintf(os.Stderr, "# warmup: %d ok, %d err, %s\n", r.ok, r.errs, r.wall.Round(time.Millisecond))
	}

	printHeader()

	// baseline = single-request latency from the R=1 round, used to express
	// each round's wall time as a multiple of one prompt (serialFactor).
	var baseline time.Duration

	for r := *minR; r <= *maxR; r += *step {
		res := runRound(client, *model, prompt, r, *timeout, *numCtx, *think)
		if r == 1 && res.ok > 0 {
			baseline = res.latMean
		}
		printRecord(res, baseline)

		if r+*step <= *maxR {
			time.Sleep(*settle) // timer-backed pause between rounds
		}
	}
}

// reqResult holds the metrics for one in-round request.
type reqResult struct {
	ttft   time.Duration // fire -> first token (content or thinking)
	total  time.Duration // fire -> done
	tokens int           // streamed chunks (a token-ish proxy)
	err    error
}

// roundResult holds the folded metrics for one concurrency round.
type roundResult struct {
	concurrency int
	ok, errs    int
	peakInFlt   int32         // max simultaneously-open requests observed
	wall        time.Duration // simultaneous release -> last completion
	sumLat      time.Duration // sum of per-request totals
	latMin      time.Duration
	latMean     time.Duration
	latMax      time.Duration
	latP50      time.Duration
	ttftMean    time.Duration
	ttftP50     time.Duration
	tokens      int
	firstErr    error
}

// runRound fires the same prompt r times, released simultaneously, and folds
// the per-request metrics. The start barrier (a channel closed once every
// goroutine is parked on it) guarantees the requests leave together rather than
// trickling out as goroutines are scheduled.
func runRound(client *ollama.Client, model, prompt string, r int, timeout time.Duration, numCtx int, think bool) roundResult {
	var (
		wg       sync.WaitGroup
		ready    sync.WaitGroup // signals each goroutine has reached the barrier
		release  = make(chan struct{})
		inFlight int32
		peak     int32
		results  = make([]reqResult, r)
	)

	wg.Add(r)
	ready.Add(r)

	for i := range r {
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			ready.Done()  // "I'm parked at the barrier"
			<-release     // all goroutines wake on the same closed channel

			// Track simultaneous occupancy; record the high-water mark.
			cur := atomic.AddInt32(&inFlight, 1)
			for {
				old := atomic.LoadInt32(&peak)
				if cur <= old || atomic.CompareAndSwapInt32(&peak, old, cur) {
					break
				}
			}
			defer atomic.AddInt32(&inFlight, -1)

			start := time.Now()
			var ttft time.Duration
			var tokens int
			mark := func(string) {
				tokens++
				if ttft == 0 {
					ttft = time.Since(start)
				}
			}
			var onThink func(string)
			if think {
				onThink = mark
			}

			_, err := client.Chat(ctx, ollama.ChatRequest{
				Model:    model,
				Messages: []ollama.Message{{Role: "user", Content: prompt}},
				Think:    think,
				NumCtx:   numCtx,
			}, mark, onThink)

			results[idx] = reqResult{ttft: ttft, total: time.Since(start), tokens: tokens, err: err}
		}(i)
	}

	// Wait until every goroutine is at the barrier, then release + start the
	// wall clock in the same instant.
	ready.Wait()
	roundStart := time.Now()
	close(release)
	wg.Wait()
	wall := time.Since(roundStart)

	return foldRound(r, wall, atomic.LoadInt32(&peak), results)
}

func foldRound(concurrency int, wall time.Duration, peak int32, results []reqResult) roundResult {
	out := roundResult{concurrency: concurrency, wall: wall, peakInFlt: peak}
	lats := make([]time.Duration, 0, len(results))
	ttfts := make([]time.Duration, 0, len(results))
	for _, r := range results {
		if r.err != nil {
			out.errs++
			if out.firstErr == nil {
				out.firstErr = r.err
			}
			continue
		}
		out.ok++
		out.tokens += r.tokens
		out.sumLat += r.total
		lats = append(lats, r.total)
		ttfts = append(ttfts, r.ttft)
	}
	if len(lats) > 0 {
		slices.Sort(lats)
		out.latMin = lats[0]
		out.latMax = lats[len(lats)-1]
		out.latMean = out.sumLat / time.Duration(len(lats))
		out.latP50 = lats[len(lats)/2]
	}
	if len(ttfts) > 0 {
		var sum time.Duration
		for _, t := range ttfts {
			sum += t
		}
		out.ttftMean = sum / time.Duration(len(ttfts))
		slices.Sort(ttfts)
		out.ttftP50 = ttfts[len(ttfts)/2]
	}
	return out
}

func printHeader() {
	fmt.Printf("%-4s %-4s %-4s %-5s %-9s %-9s %-9s %-9s %-9s %-8s %-8s %-8s\n",
		"R", "ok", "err", "peak", "wall", "ttft_p50", "lat_p50", "lat_mean", "lat_max",
		"concR", "serialX", "tok/s")
	fmt.Println("--------------------------------------------------------------------------------------------------------")
}

// printRecord emits one line per round and flushes (stdout is line-buffered
// when teed, but Printf on os.Stdout writes immediately here).
func printRecord(r roundResult, baseline time.Duration) {
	// concRatio: sum of request latencies / wall. ~1 = serialized, ~R = parallel.
	concR := 0.0
	if r.wall > 0 {
		concR = float64(r.sumLat) / float64(r.wall)
	}
	// serialX: how many single-prompt latencies this round's wall equals.
	// ~1 = as fast as one prompt (concurrent); ~R = as slow as R prompts (serial).
	serialX := 0.0
	if baseline > 0 {
		serialX = float64(r.wall) / float64(baseline)
	}
	// Aggregate throughput across the whole round.
	toksPerSec := 0.0
	if r.wall > 0 {
		toksPerSec = float64(r.tokens) / r.wall.Seconds()
	}

	fmt.Printf("%-4d %-4d %-4d %-5d %-9s %-9s %-9s %-9s %-9s %-8.2f %-8.2f %-8.0f\n",
		r.concurrency, r.ok, r.errs, r.peakInFlt,
		ms(r.wall), ms(r.ttftP50), ms(r.latP50), ms(r.latMean), ms(r.latMax),
		concR, serialX, toksPerSec)

	if r.firstErr != nil {
		fmt.Printf("     ! first error: %v\n", r.firstErr)
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%dms", d.Milliseconds()) }
