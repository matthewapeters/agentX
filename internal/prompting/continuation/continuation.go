// Package continuation detects when the agent's own response states an intent to
// keep investigating ("Let me examine the source code...", "Should I check the
// config?", "Shall I dig into the executor?") that would otherwise silently never
// happen — the turn finalizes on `err == nil` alone, with no inspection of what the
// response text actually says (sessions clever-raven-3, amber-quartz: the model
// correctly recognized its own investigation was incomplete, said so, and the turn
// just ended there).
//
// Detection is a deliberately cheap, deterministic two-step process rather than
// another LLM classification pass (matching this codebase's existing preference —
// SimilarGoals, the retry-then-degrade guard): find the trigger phrase and capture
// the verb, then check the verb against an externalized, human-editable allow/deny
// list (agentx-continuation-verbs-allowed.md / -denied.md) — the same "tune without a
// rebuild" philosophy as agentx-planner.md and agentx-classification.md. An unknown
// verb (neither list) is not guessed at; the caller asks the user once and, on an
// "always" decision, appends it to the appropriate list so the system learns without
// anyone having to predict every possible verb in advance.
package continuation

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// trigger matches a stated continuation intent and captures the verb: "Let me
// examine", "Should I check", "Shall I dig" (captures "dig" from "dig into" — the
// list is checked on the single verb word, not the full phrasal verb).
var trigger = regexp.MustCompile(`(?i)\b(?:let me|should i|shall i)\s+(\w+)`)

// Detect looks for a stated continuation intent in the LAST sentence of response
// only — not anywhere in the middle of the text — since that's what distinguishes
// "I'm about to keep going" (the turn's final word) from a rhetorical aside earlier
// in the explanation. Returns ok=false when no trigger phrase is found in that last
// sentence.
func Detect(response string) (verb, sentence string, ok bool) {
	sentence = lastSentence(response)
	if sentence == "" {
		return "", "", false
	}
	m := trigger.FindStringSubmatch(sentence)
	if m == nil {
		return "", "", false
	}
	return strings.ToLower(m[1]), sentence, true
}

// lastSentence returns the last non-empty, trimmed sentence in s, splitting on
// '.', '!', and '?'. A cheap heuristic, not a real sentence tokenizer — sufficient
// for finding "the model's final sentence", which is all Detect needs.
func lastSentence(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(parts[i]); t != "" {
			return t
		}
	}
	return ""
}

// LoadVerbs reads an allow/deny list file (one verb per line; blank lines and lines
// starting with # are ignored) into a lowercased set. A missing file is treated as
// an empty list, not an error — matching ReadPromptFile's tolerant-of-absence
// convention for every other externalized agentx-*.md file.
func LoadVerbs(path string) (map[string]bool, error) {
	if path == "" {
		return map[string]bool{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer f.Close()

	verbs := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verbs[strings.ToLower(line)] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return verbs, nil
}

// AppendVerb adds verb as a new line to the list at path, creating the file (and its
// directory) if it doesn't exist yet. Used when the user approves an unrecognized
// verb "always" — the system learning a new entry without predicting it in advance.
func AppendVerb(path, verb string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(strings.ToLower(strings.TrimSpace(verb)) + "\n")
	return err
}
