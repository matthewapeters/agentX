package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"agentx/internal/session"
)

// maxLastPromptDisplayLen bounds how much of a session's last prompt is
// shown per line in the resume picker.
const maxLastPromptDisplayLen = 60

// ResolveResume resolves a --resume flag's target to a concrete session ID.
// target == "" means "show a picker" — unless there is exactly one
// resumable session, in which case it resumes that one directly, matching
// "last"'s behavior for the degenerate single-candidate case. target ==
// "last" always resumes the most recent session without prompting.
// Anything else is matched against a candidate's session ID or name; no
// match is an error, never a silent fallback to creating a new session
// under that name (docs/architecture/behavior/session_resume.feature.md
// §2). in/out drive the picker prompt when one is needed.
func ResolveResume(store *session.Store, target string, in io.Reader, out io.Writer) (string, error) {
	candidates, err := store.ListResumable(0)
	if err != nil {
		return "", fmt.Errorf("list resumable sessions: %w", err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no resumable sessions found")
	}

	switch {
	case target == "last":
		return candidates[0].ID, nil
	case target == "" && len(candidates) == 1:
		return candidates[0].ID, nil
	case target == "":
		return promptResumePicker(candidates, in, out)
	default:
		for _, c := range candidates {
			if c.ID == target || c.Name == target {
				return c.ID, nil
			}
		}
		return "", fmt.Errorf("no resumable session matches %q", target)
	}
}

// promptResumePicker prints a numbered list (name + last prompt,
// most-recent-first — ListResumable already sorts it that way) and reads a
// selection from in.
func promptResumePicker(candidates []session.ResumeCandidate, in io.Reader, out io.Writer) (string, error) {
	for i, c := range candidates {
		fmt.Fprintf(out, "%3d. %-24s %q\n", i+1, c.Name, truncateForDisplay(c.LastPrompt, maxLastPromptDisplayLen))
	}
	fmt.Fprintf(out, "Resume which session? [1-%d]: ", len(candidates))

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read selection: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(candidates) {
		return "", fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
	return candidates[n-1].ID, nil
}

// truncateForDisplay caps s to max runes for the picker's one-line-per-
// session display, appending an ellipsis when it does.
func truncateForDisplay(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
