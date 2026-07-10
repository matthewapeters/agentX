# Continuation verbs — allow-list
#
# When the agent's own response ends with "Let me <verb> ...", "Should I <verb> ...?",
# or "Shall I <verb> ...?", the verb is checked against this list. A verb here is
# treated as a genuine investigative continuation: the turn runs one more bounded
# decomposition round (grounded in what this plan has already found — see
# internal/planfindings) before finalizing, instead of silently ending on a stated
# intent that never happens.
#
# One verb per line. Lines starting with # and blank lines are ignored. Matching is
# case-insensitive. Edit this file directly to add or remove verbs — no rebuild
# needed. Approving an unrecognized verb "always" from the chat surface appends it
# here automatically.
#
# NOTE: `make install`'s seed step currently overwrites this file unconditionally on
# every reinstall — retention of a customized/learned list across reinstalls is not
# yet implemented (the same limitation every other agentx-*.md seed file already has).

examine
dig
check
look
investigate
review
analyze
verify
read
search
explore
