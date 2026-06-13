package main

import "testing"

// GIVEN tmux session startup is building the initial command
// WHEN the command is generated for first attach view
// THEN window 0 should be explicitly named tui-chat for UX parity.
func TestBuildNewSessionCommand_NamesPrimaryWindowTUIChat(t *testing.T) {
	session := "agentx_test"
	got := buildNewSessionCommand(session, 120, 40)

	found := false
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "-n" && got[i+1] == tmuxPrimaryWindow {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected new session command to include primary window name '-n %s', got %v", tmuxPrimaryWindow, got)
	}
}

func TestBuildNewSessionCommand(t *testing.T) {
	session := "agentx_test"
	got := buildNewSessionCommand(session, 120, 40)
	want := []string{"new-session", "-d", "-s", session, "-n", tmuxPrimaryWindow, "-x", "120", "-y", "40"}

	if len(got) != len(want) {
		t.Fatalf("new session command length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("new session command arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestResolveStartupWindowSize_UsesEnvironmentWithMinimums(t *testing.T) {
	t.Setenv("LINES", "28")
	t.Setenv("COLUMNS", "92")

	width, height := resolveStartupWindowSize(nil)
	if width != 92 {
		t.Fatalf("expected startup width 92, got %d", width)
	}
	if height != 28 {
		t.Fatalf("expected startup height 28, got %d", height)
	}
}

func TestResolveStartupWindowSize_ClampsMinimums(t *testing.T) {
	t.Setenv("LINES", "12")
	t.Setenv("COLUMNS", "60")

	width, height := resolveStartupWindowSize(nil)
	if width != minStartupWindowWidth {
		t.Fatalf("expected startup width clamp %d, got %d", minStartupWindowWidth, width)
	}
	if height != minStartupWindowHeight {
		t.Fatalf("expected startup height clamp %d, got %d", minStartupWindowHeight, height)
	}
}

func TestBuildTmuxSessionName_SanitizesComponents(t *testing.T) {
	got := buildTmuxSessionName("User Name/Team", "Session 42@Dev")
	want := "agentx_user-name-team_session-42-dev"
	if got != want {
		t.Fatalf("tmux session name mismatch: got %q want %q", got, want)
	}
}

func TestBuildTmuxSessionName_UsesFallbacksForEmptyComponents(t *testing.T) {
	got := buildTmuxSessionName("   ", "$$$")
	want := "agentx_user_session"
	if got != want {
		t.Fatalf("tmux session fallback mismatch: got %q want %q", got, want)
	}
}

func TestSplitCommandsUsePaneIDCapture(t *testing.T) {
	chatPane := "agentx_test:0.0"

	inputSplit := buildInputSplitCommand(chatPane)
	contextSplit := buildContextSplitCommand(chatPane)

	if len(inputSplit) < 4 || inputSplit[1] != "-P" || inputSplit[2] != "-F" || inputSplit[3] != "#{pane_id}" {
		t.Fatalf("input split command must capture pane_id, got %v", inputSplit)
	}
	if len(contextSplit) < 4 || contextSplit[1] != "-P" || contextSplit[2] != "-F" || contextSplit[3] != "#{pane_id}" {
		t.Fatalf("context split command must capture pane_id, got %v", contextSplit)
	}
}

func TestPaneTargets_MapsAllPanesCorrectly(t *testing.T) {
	targets := paneTargets("agentx_test", "%1", "%3", "%4")
	if len(targets) != 4 {
		t.Fatalf("expected 4 pane targets, got %d", len(targets))
	}

	want := map[string]string{
		PaneTitleOutput: "%1",
		PaneTitleInput:  "%3",
		PaneTitleSystem: "%4",
		PaneTitleLogs:   "agentx_test:1.0",
	}

	for _, target := range targets {
		wantTarget, ok := want[target.name]
		if !ok {
			t.Fatalf("unexpected pane target name %q", target.name)
		}
		if target.target != wantTarget {
			t.Fatalf("pane %q target mismatch: got %q want %q", target.name, target.target, wantTarget)
		}
	}
}
