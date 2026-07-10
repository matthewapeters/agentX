# Continuation verbs — deny-list (blacklist)
#
# A verb here is the opposite of agentx-continuation-verbs-allowed.md: "Let me
# <verb> ..." (or "Should/Shall I <verb> ...?") ending a response is recognized but
# deliberately NOT treated as a continuation trigger, and the surface is not asked
# about it again. Otherwise starts empty — populated further only when the user
# explicitly chooses "never" for a verb the surface asked about.
#
# Pre-seeded with a handful of common closing pleasantries ("let me know if you have
# questions", "let me explain") that are grammatically identical to a stated
# investigative intent but mean something else entirely — without this, the very
# first "let me know" in a normal reply would trigger an unnecessary approval prompt.
# Remove any of these if you'd rather be asked.
#
# One verb per line. Lines starting with # and blank lines are ignored. Matching is
# case-insensitive.

know
clarify
explain
summarize
recap
