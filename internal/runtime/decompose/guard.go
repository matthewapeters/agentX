package decompose

import (
	"regexp"
	"strings"
)

// The tidy-cove spiral (ADR 0009): the planner phrased steps as "Run `ls -la` … and
// capture its output", ten levels of re-planning producing the same goal rephrased.
// SimilarGoals detects that echo so the decomposer can refuse to recurse
// (scheduler.ErrNoProgress → the node executes as a Task instead). stripResultPlumbing
// feeds it: plumbing clauses ("and save output to $X") are stripped before comparison so
// they cannot mask an otherwise-identical goal — the executor returns results
// automatically, so plumbing carries no distinguishing content.

// resultPlumbing matches a trailing result-capture clause: "and/then capture|save|store|
// write|return|record|print|pipe … output|result|stdout|stderr …" up to the next clause
// boundary. Case-insensitive; the object may precede ("capture its output") or the verb may
// be bare before a $VAR/file target.
var resultPlumbing = regexp.MustCompile(
	`(?i)(?:,?\s+(?:and|then)\s+)(?:capture|save|store|write|return|record|print|pipe)\b[^,;.]*`)

// stripResultPlumbing removes result-capture clauses from a goal, returning the action
// that remains.
func stripResultPlumbing(goal string) string {
	return strings.TrimSpace(resultPlumbing.ReplaceAllString(goal, ""))
}

// stopwords carry no goal identity and are dropped before comparison, so filler
// ("on", "the") cannot dilute an echo below the similarity threshold.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "on": true, "in": true, "at": true, "to": true,
	"of": true, "and": true, "then": true, "its": true, "it": true, "all": true,
	"that": true, "this": true, "with": true, "for": true, "from": true, "into": true,
}

// verbSynonyms folds interchangeable action verbs to one canonical form — the tidy-cove
// chain alternated "Run"/"Execute" for the identical command, which must not read as a
// different goal.
var verbSynonyms = map[string]string{
	"execute": "run", "invoke": "run", "launch": "run",
	"display": "show", "print": "show",
	"enumerate": "list",
	"examine": "inspect", "look": "inspect",
}

// goalTokens normalizes a goal to a comparable token set: plumbing stripped, lower-cased,
// backticks/punctuation dropped, stopwords removed, verb synonyms folded.
func goalTokens(goal string) map[string]bool {
	clean := strings.ToLower(stripResultPlumbing(goal))
	clean = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ', r == '/', r == '.':
			return r
		default:
			return ' '
		}
	}, clean)
	toks := map[string]bool{}
	for t := range strings.FieldsSeq(clean) {
		if stopwords[t] {
			continue
		}
		if canon, ok := verbSynonyms[t]; ok {
			t = canon
		}
		toks[t] = true
	}
	return toks
}

// SimilarGoals reports whether two goals are near-identical once normalized — the echo
// signature of a non-progress decomposition. Overlap is measured against the smaller set
// (containment), so a rephrasing that only adds plumbing words still matches.
func SimilarGoals(a, b string) bool {
	ta, tb := goalTokens(a), goalTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	small, large := ta, tb
	if len(tb) < len(ta) {
		small, large = tb, ta
	}
	shared := 0
	for t := range small {
		if large[t] {
			shared++
		}
	}
	return float64(shared)/float64(len(small)) >= 0.8
}
