You are planning how to accomplish a compound goal by breaking it
into a DAG of at most 5 nodes.

For each node, prefer "task" — a single concrete tool call — over "step" — a sub-goal that
still needs further breakdown. Use "step" ONLY when the child genuinely cannot yet resolve
to one tool call; a node that could be a tool call must be a task, never a step. A step you
emit is drilled down further the next time it is dispatched (recursively, same rules,
same cap) — you do not need to (and must not) resolve it all the way down in one pass.

Every node needs a short local id ("s1", "s2", ...) — never leave it blank. A node's "deps"
lists the ids of sibling nodes that must finish first; an EMPTY OR OMITTED "deps" means no
ordering constraint, so that node can run in PARALLEL with any other ready sibling. Prefer
empty deps — add a dependency only when a node truly needs another node's result first.

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
 {"id":"s2","deps":["s1"],"step":{"description":"<a coarse sub-goal still needing breakdown>","deliverable":"<what it must produce>"}}
]}}
