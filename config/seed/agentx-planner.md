You are planning how to accomplish a compound goal by breaking it
into a DAG of at most 5 nodes.

For each node, prefer "task" — a single concrete tool call — over "step" — a sub-goal that
still needs further breakdown. Use "step" ONLY when the child genuinely cannot yet resolve
to one tool call; a node that could be a tool call must be a task, never a step. A step you
emit is drilled down further the next time it is dispatched (recursively, same rules,
same cap) — you do not need to (and must not) resolve it all the way down in one pass.

Every node needs a short local id ("s1", "s2", ...) — never leave it blank. A node's "deps"
lists the ids of sibling nodes that must finish first. Most data-gathering nodes (list, read,
explore) have no deps and run in PARALLEL — leave "deps" empty for those.

But check every node's own explanation/description for words like "compile", "combine",
"summarize", "synthesize", or "compare" — if it says it works from OTHER nodes' results, it is
an aggregation node, and "deps" MUST list EVERY sibling it draws from, not just one. This rule
applies to "step" nodes exactly as much as "task" nodes — a step is still dispatched (and, for
a step, decomposed further) as soon as its listed deps finish, so an aggregation step with
empty deps starts decomposing before the results it needs to work from even exist, same as a
task would. When unsure whether a dependency is needed, add it: a missing dependency breaks
the plan, an unneeded one only costs a little parallelism. (See the third node, "s3", in the
JSON example below — that is exactly this pattern, worked correctly.)

Do not add a node that writes a report, summary, or findings to a file. The system
automatically composes the final answer for the user from every completed node's results once
the plan finishes — your nodes only gather or transform information, they never produce the
final write-up themselves.

Never restate the goal itself as a node. If the goal is already a single concrete action,
reply with exactly one task node naming that action directly.

Tools available for "task" nodes (use "tool" and "args" from this list only):
{{catalog}}
No shell syntax in args: no pipes, redirects, $VARIABLES, or command chaining.

Only use paths/facts you actually know from "What you know" below. Do not invent a path
that isn't given to you — prefer a task that lists a directory to discover real filenames
before a task that reads one.

What you know:
{{context}}

Goal:
{{goal}}

Reply with JSON matching this shape exactly (ids/paths below are illustrative, not to be
copied):
{"plan":{"name":"<3-6 words>","objective":"<one sentence>","dag":[
 {"id":"s1","deps":[],"task":{"tool":"list_dir","args":{"path":"."},"explanation":"<why>"}},
 {"id":"s2","deps":[],"step":{"description":"<a coarse sub-goal still needing breakdown, independent of s1, so it also runs in parallel>","deliverable":"<what it must produce>"}},
 {"id":"s3","deps":["s1","s2"],"step":{"description":"<compares/combines/synthesizes s1's AND s2's results into ...>","deliverable":"<the combined output>"}}
]}}
