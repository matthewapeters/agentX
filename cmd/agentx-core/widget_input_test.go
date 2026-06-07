package main

import "testing"

func TestNormalizeWidgetControlCommand(t *testing.T) {
	aliases := map[string]string{
		"?":        "help",
		"controls": "help",
		"exit":     "quit",
	}

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "trim and strip colon", raw: "  :help  ", want: "help"},
		{name: "question alias", raw: ":?", want: "help"},
		{name: "controls alias", raw: "controls", want: "help"},
		{name: "exit alias", raw: "exit", want: "quit"},
		{name: "navigation token", raw: "up", want: "k"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeWidgetControlCommand(tc.raw, aliases); got != tc.want {
				t.Fatalf("normalizeWidgetControlCommand(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDefaultWidgetControlAliases(t *testing.T) {
	aliases := defaultWidgetControlAliases()
	if aliases["?"] != "help" {
		t.Fatalf("expected ? alias to map to help, got %q", aliases["?"])
	}
	if aliases["controls"] != "help" {
		t.Fatalf("expected controls alias to map to help, got %q", aliases["controls"])
	}
	if aliases["exit"] != "quit" {
		t.Fatalf("expected exit alias to map to quit, got %q", aliases["exit"])
	}
}

func TestHandleWidgetLoopControlCommand(t *testing.T) {
	helpCalls := 0
	refreshCalls := 0

	if action := handleWidgetLoopControlCommand("quit", widgetLoopControlHandlers{
		QuitTokens: []string{"q", "quit"},
	}); action != widgetLoopControlQuit {
		t.Fatalf("expected quit action, got %v", action)
	}

	if action := handleWidgetLoopControlCommand("help", widgetLoopControlHandlers{
		HelpTokens: []string{"help"},
		OnHelp: func() {
			helpCalls++
		},
	}); action != widgetLoopControlHandled {
		t.Fatalf("expected handled action for help, got %v", action)
	}

	if action := handleWidgetLoopControlCommand("refresh", widgetLoopControlHandlers{
		RefreshTokens: []string{"r", "refresh"},
		OnRefresh: func() {
			refreshCalls++
		},
	}); action != widgetLoopControlHandled {
		t.Fatalf("expected handled action for refresh, got %v", action)
	}

	if action := handleWidgetLoopControlCommand("noop", widgetLoopControlHandlers{
		QuitTokens: []string{"q", "quit"},
	}); action != widgetLoopControlNone {
		t.Fatalf("expected none action for unknown command, got %v", action)
	}

	if helpCalls != 1 {
		t.Fatalf("expected help callback once, got %d", helpCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("expected refresh callback once, got %d", refreshCalls)
	}
}

func TestNormalizeWidgetEscapeSequence_ShiftTab(t *testing.T) {
	tests := []string{"\x1b[Z", "\x1b[1;2Z"}
	for _, raw := range tests {
		command, ok := normalizeWidgetEscapeSequence(raw)
		if !ok {
			t.Fatalf("expected shift-tab sequence %q to normalize", raw)
		}
		if command != "shift-tab" {
			t.Fatalf("expected shift-tab for %q, got %q", raw, command)
		}
	}
}

func TestNormalizeWidgetControlCommand_ShiftTabEscapePassthrough(t *testing.T) {
	if got := normalizeWidgetControlCommand("\x1b[Z", nil); got != "shift-tab" {
		t.Fatalf("expected shift-tab from escape, got %q", got)
	}
}
