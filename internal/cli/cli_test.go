package cli

import "testing"

// GIVEN --resume with no following argument
// WHEN Parse runs
// THEN Resume is true and ResumeTarget is empty — signals "show a picker."
func TestParseResumeBare(t *testing.T) {
	cmd, err := Parse([]string{"--resume"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cmd.Resume {
		t.Error("Resume = false, want true")
	}
	if cmd.ResumeTarget != "" {
		t.Errorf("ResumeTarget = %q, want empty", cmd.ResumeTarget)
	}
}

// GIVEN --resume followed by a session name/ID
// WHEN Parse runs
// THEN ResumeTarget carries that value.
func TestParseResumeWithValue(t *testing.T) {
	cmd, err := Parse([]string{"--resume", "eager-enchanting-mango"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cmd.Resume || cmd.ResumeTarget != "eager-enchanting-mango" {
		t.Errorf("Resume=%v ResumeTarget=%q, want true/eager-enchanting-mango", cmd.Resume, cmd.ResumeTarget)
	}
}

// GIVEN --resume=<value>
// WHEN Parse runs
// THEN ResumeTarget carries that value — the = form works the same as the
// space-separated one.
func TestParseResumeEqualsForm(t *testing.T) {
	cmd, err := Parse([]string{"--resume=last"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cmd.Resume || cmd.ResumeTarget != "last" {
		t.Errorf("Resume=%v ResumeTarget=%q, want true/last", cmd.Resume, cmd.ResumeTarget)
	}
}

// GIVEN --resume immediately followed by another flag (no session name in
// between)
// WHEN Parse runs
// THEN ResumeTarget stays empty and the following flag is parsed normally —
// --resume's optional value must not swallow the next flag as if it were a
// session name.
func TestParseResumeDoesNotConsumeFollowingFlag(t *testing.T) {
	cmd, err := Parse([]string{"--resume", "--version"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.ResumeTarget != "" {
		t.Errorf("ResumeTarget = %q, want empty (must not consume --version as a value)", cmd.ResumeTarget)
	}
	if !cmd.ShowVersion {
		t.Error("ShowVersion = false, want true (--version must still be parsed after --resume)")
	}
}

// GIVEN --resume combined with --session
// WHEN Parse runs
// THEN both are recorded independently — resuming an existing session and
// naming it are orthogonal flags.
func TestParseResumeWithSessionFlag(t *testing.T) {
	cmd, err := Parse([]string{"--resume", "my-old-session", "--session", "new-name"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.ResumeTarget != "my-old-session" {
		t.Errorf("ResumeTarget = %q, want my-old-session", cmd.ResumeTarget)
	}
	if cmd.SessionName != "new-name" {
		t.Errorf("SessionName = %q, want new-name", cmd.SessionName)
	}
}
