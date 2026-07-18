// Package wavefront is ADR 0012's round-synchronized decomposition engine: a
// sibling to internal/runtime/decompose and internal/runtime/scheduler, selectable
// alongside them rather than replacing either. This file (Phase 2 of the ADR's
// Phased Build Plan) holds only prompt scaffolding — default templates and their
// render functions — with no Blackboard, Classifier, or Scheduler yet; those land
// in later phases as sibling files in this same package.
//
// Design: docs/architecture/adr/0012-wavefront-grounded-decomposition-and-shared-blackboard.md.
package wavefront

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultClassifyPromptTemplate is the built-in wavefront classify SYSTEM prompt,
// used when no agentx-wavefront-classify.md is configured
// (config/seed/agentx-wavefront-classify.md ships the same content as the editable
// seed — this constant is only the fallback, mirroring
// planner.DefaultPromptTemplate's convention). {{catalog}} is the compact tool
// catalog. Durable, call-independent rules only — working memory and the question
// are the USER message (DefaultClassifyUserTemplate below), per the same
// system/user split ADR 0011 established for the planner.
const DefaultClassifyPromptTemplate = `You are resolving one open question as part of a larger investigation. You will be told
what is already known (WORKING MEMORY) and asked to classify everything relevant into three
kinds:

- **KNOW** — a fact you already have, or can derive purely by combining facts already
  given to you in WORKING MEMORY (synthesis). Never a fact you are guessing or assuming.
- **TOOL** — something required to answer the question that you can resolve immediately,
  because you already know, from WORKING MEMORY alone, a concrete tool call from the
  catalog below that would directly resolve it — including every argument the tool
  requires. The arguments must themselves come from WORKING MEMORY or the question itself
  — never from a fact you expect another item in this same response to produce. If
  resolving this would require something you do not yet have (a filename you have not
  seen, a path you have not confirmed), that unresolved thing is a **NEED**, not a TOOL —
  do not guess it just to fill in an argument.
- **NEED** — something required to answer the question that is not yet in WORKING MEMORY,
  and that you cannot yet name a complete tool call for. This becomes a new question,
  asked again once more is known.

Prefer TOOL over NEED whenever the catalog genuinely gives you everything the tool needs
*right now* — resolving something immediately is cheaper than leaving it open for a later
round. But never let that preference push you into inventing an argument you do not
actually have, or omitting one the tool requires. A correct NEED that gets resolved next
round is strictly better than a TOOL with a guessed or incomplete argument — a wrong or
incomplete call wastes the round it runs in (or is silently rejected before it ever runs)
and can mislead everything downstream of it.

Tools available (use "tool" and "args" from this list only, and supply every argument the
tool requires — never fewer):
{{catalog}}
No shell syntax in args: no pipes, redirects, $VARIABLES, or command chaining.

NAMING DISCIPLINE: every KNOW/NEED/TOOL "name" must precisely describe that one piece of
information — never a vague paraphrase. The same information must always be named
identically if it recurs. WORKING MEMORY may list names of information already being
investigated elsewhere in this problem ("currently open"); if what you need is the exact
same information as one of those, reuse its name exactly rather than inventing a new one —
this lets two independent lines of investigation converge on one answer instead of each
re-deriving it. Only reuse a name when the information is truly identical, never for
something merely similar.

If your synthesis fully answers the question exactly as asked (not a sub-part of it), use
the question's own text, verbatim, as the "name" of that KNOW item.

There is no minimum or maximum number of NEED/TOOL items — if the question is already fully
answerable from WORKING MEMORY, return zero. Otherwise be thorough: identify every distinct
piece of information you can in this one pass, rather than one item now and the rest next
round.

----
EXAMPLE (a TOOL whose argument comes from WORKING MEMORY, not a guess; a NEED for
something not yet known):

WORKING MEMORY:
project_folder: /home/example/demo-project
directory listing of /home/example/demo-project: [go.mod, cmd/, internal/, README.md]

QUESTION:
----
What does this project's README say about how to run it?
----

RESPONSE:
{"classification": [
{"KNOW": {"name": "README.md exists at the project root", "value": "true — seen in the directory listing already in WORKING MEMORY"}},
{"TOOL": {"name": "contents of README.md", "tool": "read_file", "args": {"path": "/home/example/demo-project/README.md"}}}
]}

Note what this example does NOT do: it does not include a KNOW item claiming to already
know the README's contents, and it does not invent a "run instructions" NEED naming a
file or section it has not confirmed exists — that would be exactly the kind of guess this
prompt exists to prevent.

----
SECOND EXAMPLE (a tool call needing two arguments — both must be present, or this must be
a NEED instead):

WORKING MEMORY:
project_folder: /home/example/demo-project

QUESTION:
----
What files exist under this project's internal/ directory?
----

RESPONSE:
{"classification": [
{"TOOL": {"name": "contents of /home/example/demo-project/internal", "tool": "list_dir", "args": {"path": "/home/example/demo-project/internal"}}}
]}

If WORKING MEMORY had not established the project's path, "path" could not be filled in
without guessing — the correct classification would then be a NEED naming the missing
information ("the project's root directory"), not a TOOL with a fabricated or omitted
argument. A TOOL item with an argument left out is not a smaller or safer version of the
call: it is rejected before it ever runs, identically to a call this prompt itself
disallows.`

// DefaultClassifyUserTemplate is the built-in wavefront classify USER message:
// per-call data (working memory, the question) plus the reply-format spec —
// mechanical scaffolding, not tunable guidance, so unlike
// DefaultClassifyPromptTemplate it has no seed-file counterpart (mirrors
// planner.DefaultUserTemplate's same choice, ADR 0011). The reply-format spec stays
// adjacent to the question deliberately, same rationale as planner.DefaultUserTemplate:
// recency right before generation is wanted here, so the model remembers the exact
// output shape right as it responds.
const DefaultClassifyUserTemplate = `WORKING MEMORY:
{{wm}}

QUESTION:
----
{{question}}
----

Reply with JSON matching this shape exactly (names/values below are illustrative,
not to be copied):
{"classification": [
 {"KNOW": {"name": "<precise name>", "value": "<fact or synthesis, from WORKING MEMORY only>"}},
 {"TOOL": {"name": "<precise name>", "tool": "<tool id>", "args": {"<arg>": "<value>"}}},
 {"NEED": {"name": "<precise name>"}}
]}`

// DefaultSynthesisPromptTemplate is the built-in wavefront synthesis SYSTEM prompt
// (config/seed/agentx-wavefront-synthesis.md ships the same content). Used once a
// question's own leaves are exhausted with no KNOW matching it verbatim — a
// dedicated, schema-free call, mirroring totAlX's two-phase decompose-then-
// synthesize fix (see ADR 0012 §4).
const DefaultSynthesisPromptTemplate = `You have already gathered enough information to fully answer the question below — no
further information is needed. Synthesize a direct, complete answer using only the
information already given to you. Respond in plain prose. Do not use JSON, markdown
fences, or any other structured format — just the answer itself.

If the given information is genuinely insufficient to answer confidently, say so plainly
rather than filling the gap with a guess.`

// DefaultSynthesisUserTemplate is the built-in wavefront synthesis USER message.
const DefaultSynthesisUserTemplate = `Answer the following using only the information given:
----
{{question}}
----
WORKING MEMORY:
{{wm}}`

// DefaultSummaryPromptTemplate is the built-in wavefront output-summarization
// SYSTEM prompt (config/seed/agentx-wavefront-summary.md ships the same content).
// {{target_chars}} is filled per call (RenderSummarySystem below) — it is a
// numeric constraint on the response, not per-call task data, so it stays in the
// system message rather than the user message. Chain-aware and schema-free,
// targeted at the most specific item in the question chain only (ADR 0012 §6).
const DefaultSummaryPromptTemplate = `You will be given the raw output of a command or tool, along with the chain of questions
that led to running it — from the most general (the original problem) to the most
specific (exactly what this command was meant to determine). Extract and condense, from
the raw output, only the information relevant to the MOST SPECIFIC item in that chain,
into a plain-prose summary of no more than about {{target_chars}} characters.

Use the more general items in the chain only as context to help judge what is relevant —
do not attempt to answer the general question directly, and do not add information that
is not present in the raw output. Do not use JSON or markdown fences; plain prose only.`

// DefaultSummaryUserTemplate is the built-in wavefront output-summarization USER
// message: the question chain (general to specific) and the raw output to condense.
const DefaultSummaryUserTemplate = `QUESTION CHAIN (general to specific):
{{chain}}

RAW OUTPUT (relevant to the most specific item):
----
{{output}}
----`

// RenderClassifySystem fills template (DefaultClassifyPromptTemplate, or the
// configured agentx-wavefront-classify.md content) for the tool catalog.
func RenderClassifySystem(template, catalog string) string {
	return strings.ReplaceAll(template, "{{catalog}}", catalog)
}

// RenderClassifyUser fills template (DefaultClassifyUserTemplate) for the current
// working-memory text and the question being classified.
func RenderClassifyUser(template, wm, question string) string {
	r := strings.NewReplacer("{{wm}}", wm, "{{question}}", question)
	return r.Replace(template)
}

// RenderToolRetryFeedback formats one or more invalid TOOL proposals into the
// feedback block a classify retry appends to the original question: what was
// proposed, exactly why it was rejected, and the tool's full contract, so the
// model can correct the call or fall back to NEED instead of guessing again
// blind. The original question and working memory are never altered by this —
// it is additive text the caller appends, so a retry always sees its full
// original context plus, on top of it, what specifically failed last time.
func RenderToolRetryFeedback(failures []ToolFailure) string {
	var b strings.Builder
	b.WriteString("\n\n[Your previous response had invalid TOOL call(s). Reclassify this exact question again, " +
		"fixing these — or use NEED instead if you don't actually have the missing value yet:]\n")
	for _, f := range failures {
		fmt.Fprintf(&b, "- TOOL %q (tool %q) is malformed: %s. Tool contract: %s.\n", f.Name, f.Tool, f.Err, f.Contract)
	}
	return b.String()
}

// RenderSynthesisUser fills template (DefaultSynthesisUserTemplate) for the
// working-memory text and the question to answer.
func RenderSynthesisUser(template, wm, question string) string {
	r := strings.NewReplacer("{{wm}}", wm, "{{question}}", question)
	return r.Replace(template)
}

// RenderSummarySystem fills template (DefaultSummaryPromptTemplate, or the
// configured agentx-wavefront-summary.md content) for the target character budget.
func RenderSummarySystem(template string, targetChars int) string {
	return strings.ReplaceAll(template, "{{target_chars}}", strconv.Itoa(targetChars))
}

// RenderSummaryUser fills template (DefaultSummaryUserTemplate) for the
// general-to-specific question chain and the raw output to condense.
func RenderSummaryUser(template, chain, output string) string {
	r := strings.NewReplacer("{{chain}}", chain, "{{output}}", output)
	return r.Replace(template)
}
