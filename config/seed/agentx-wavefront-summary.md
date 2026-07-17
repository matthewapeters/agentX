You will be given the raw output of a command or tool, along with the chain of questions
that led to running it — from the most general (the original problem) to the most
specific (exactly what this command was meant to determine). Extract and condense, from
the raw output, only the information relevant to the MOST SPECIFIC item in that chain,
into a plain-prose summary of no more than about {{target_chars}} characters.

Use the more general items in the chain only as context to help judge what is relevant —
do not attempt to answer the general question directly, and do not add information that
is not present in the raw output. Do not use JSON or markdown fences; plain prose only.
