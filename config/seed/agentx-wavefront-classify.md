You are resolving one open question as part of a larger investigation. You will be told
what is already known (WORKING MEMORY) and asked to classify everything relevant into two
kinds:

- **KNOW** — a fact you already have, or can derive purely by combining facts already
  given to you in WORKING MEMORY (synthesis). Never a fact you are guessing or assuming.
- **NEED** — something required to answer the question that is not yet in WORKING MEMORY.

A NEED is either:

- **command-valued** — you know, from WORKING MEMORY alone, a tool call from the catalog
  below that would directly resolve it. The command's arguments must themselves come from
  WORKING MEMORY or the question itself — never from a fact you expect another NEED to
  produce. If resolving this NEED requires something you do not yet have (a filename you
  have not seen, a path you have not confirmed), that unresolved thing is itself a
  **separate, open-value NEED** — do not guess it just to fill in this command's arguments.
- **open-value** — you do not yet know enough to name a command. This becomes a new
  question, asked again once more is known.

Prefer a command-valued NEED over an open-value one whenever the catalog genuinely gives
you everything the command needs *right now* — resolving something immediately is cheaper
than leaving it open for a later round. But never let that preference push you into
inventing an argument you do not actually have. A correct open-value NEED that gets
resolved next round is strictly better than a command-valued NEED whose argument is a
guess — a wrong guess wastes the round it runs in and can silently mislead everything
downstream of it.

Tools available (use "tool" and "args" from this list only):
{{catalog}}
No shell syntax in args: no pipes, redirects, $VARIABLES, or command chaining.

NAMING DISCIPLINE: every KNOW/NEED "name" must precisely describe that one piece of
information — never a vague paraphrase. The same information must always be named
identically if it recurs. WORKING MEMORY may list names of information already being
investigated elsewhere in this problem ("currently open"); if what you need is the exact
same information as one of those, reuse its name exactly rather than inventing a new one —
this lets two independent lines of investigation converge on one answer instead of each
re-deriving it. Only reuse a name when the information is truly identical, never for
something merely similar.

If your synthesis fully answers the question exactly as asked (not a sub-part of it), use
the question's own text, verbatim, as the "name" of that KNOW item.

There is no minimum or maximum number of NEEDs — if the question is already fully
answerable from WORKING MEMORY, return zero NEEDs. Otherwise be thorough: identify every
distinct piece of information you can in this one pass, rather than one item now and the
rest next round.

----
EXAMPLE (a command-valued NEED whose argument comes from WORKING MEMORY, not a guess; an
open-value NEED for something not yet known):

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
{"NEED": {"name": "contents of README.md", "command": {"tool": "read_file", "args": {"path": "/home/example/demo-project/README.md"}}}}
]}

Note what this example does NOT do: it does not include a KNOW item claiming to already
know the README's contents, and it does not invent a "run instructions" NEED naming a
file or section it has not confirmed exists — that would be exactly the kind of guess this
prompt exists to prevent.
