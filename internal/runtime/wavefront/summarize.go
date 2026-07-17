package wavefront

import (
	"context"
	"fmt"
	"strings"
)

// OutputSummaryThreshold is the character length above which oversized findings are
// summarized rather than used as-is (ADR 0012 §6) — most tool output is small and
// stays under this, matching totAlX's own "common case" design. A character count,
// not a line count: a single very long line (a minified file, a raw JSON blob) can
// blow this budget while still reading as "one line" to a line-based cap. Ported
// from totAlX's own measurement as an illustrative starting point, not a tuned
// agentX constant. Owned here (not by the continuous-engine caller), and moved from
// internal/runtime in Phase 7a: wavefront needs it too for command-Need results, and
// internal/runtime already imports internal/runtime/wavefront (since Phase 3, for
// the prompt scaffolding below), so the reverse import would cycle — the mechanics
// have to live wherever the prompts already do.
const OutputSummaryThreshold = 2000

// OutputSummaryTargetChars is the requested size of a summarized finding (guidance
// handed to the model, not a hard cap) and the size a fallback truncation is cut to.
const OutputSummaryTargetChars = 1200

// CondenseFunc condenses text (real, oversized findings) toward
// OutputSummaryTargetChars, given the general-to-specific chain of questions that
// led to it (root goal first, the most specific question last) — targeted at the
// most specific item only, per ADR 0012 §6. Always returns usable text — it never
// errors, since a failed condense degrades to TruncateFindings internally rather
// than losing the finding.
type CondenseFunc func(ctx context.Context, chain []string, text string) string

// NewCondenser builds a CondenseFunc backed by chat (ADR 0012 §6), rendering the
// summary prompt (template overrides DefaultSummaryPromptTemplate when non-empty —
// Settings.WavefrontSummaryPrompt) with no Format constraint: this is schema-free
// plain prose with nothing structured to break, exactly the case ADR 0012 §7 says is
// safe — and preferable, for speed — to run without reasoning.
func NewCondenser(chat Chat, template string) CondenseFunc {
	sysTemplate := template
	if sysTemplate == "" {
		sysTemplate = DefaultSummaryPromptTemplate
	}
	return func(ctx context.Context, chain []string, text string) string {
		sys := RenderSummarySystem(sysTemplate, OutputSummaryTargetChars)
		usr := RenderSummaryUser(DefaultSummaryUserTemplate, renderChain(chain), text)
		if summary, err := chat(ctx, sys, usr, nil); err == nil {
			if summary = strings.TrimSpace(summary); summary != "" {
				return fmt.Sprintf("[summarized from %d chars] %s", len(text), summary)
			}
		}
		return TruncateFindings(text)
	}
}

// TruncateFindings is the last-resort fallback when summarization is unavailable or
// fails: a prefix cut, positionally arbitrary (may cut off exactly the part that
// mattered) but strictly better than an unbounded payload reaching every subsequent
// call in a plan. Rune-safe: a naive byte-index slice can split a multi-byte UTF-8
// rune at the boundary and produce invalid text. Exported so a caller with no
// CondenseFunc wired at all (e.g. capturingExec's degrade-gracefully posture for a
// nil summarizer) can still fall back to it directly.
func TruncateFindings(text string) string {
	runes := []rune(text)
	if len(runes) <= OutputSummaryTargetChars {
		return text
	}
	kept := string(runes[:OutputSummaryTargetChars])
	return fmt.Sprintf("%s... [truncated, %d more chars]", kept, len(runes)-OutputSummaryTargetChars)
}

// renderChain numbers a general-to-specific question chain for the summarization
// prompt's user message, matching totAlX's own "1. ... 2. ..." chain format.
func renderChain(chain []string) string {
	var b strings.Builder
	for i, step := range chain {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	return strings.TrimRight(b.String(), "\n")
}
