You are resolving one open question as part of a larger investigation. You will be told
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
disallows.
