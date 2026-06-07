package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatInputPromptLabel_ClipsForNarrowPane(t *testing.T) {
	t.Setenv("COLUMNS", "22")
	label := formatInputPromptLabel("agentx[thinking][ctx:very-long-context-file-name.md]", nil, false)
	if len([]rune(label)) > 20 {
		t.Fatalf("expected clipped prompt label for narrow pane, got %q", label)
	}
}

func TestPrintInputWidgetLines_WrapsToPaneWidth(t *testing.T) {
	t.Setenv("COLUMNS", "30")
	buf := &bytes.Buffer{}
	printInputWidgetLines(buf, "Multiline controls once active: :submit/:send/:done, :cancel/:discard")
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatal("expected wrapped output lines")
	}
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 29 {
			t.Fatalf("expected wrapped lines <= 29 chars, got %d for %q", len([]rune(line)), line)
		}
	}
}

func TestSubmitPromptToCore_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		if req.Prompt != "hello" {
			t.Fatalf("expected prompt hello, got %q", req.Prompt)
		}

		_ = json.NewEncoder(w).Encode(submitResponse{Response: "Echo: hello"})
	}))
	defer server.Close()

	response, err := submitPromptToCore(context.Background(), server.URL, "hello")
	if err != nil {
		t.Fatalf("submitPromptToCore returned error: %v", err)
	}
	if response != "Echo: hello" {
		t.Fatalf("expected response Echo: hello, got %q", response)
	}
}

func TestRunInputWidgetCommand_SubmitsAndQuits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString("hello\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "\x1b[1A\x1b[2K\r") {
		t.Fatalf("expected input clear escape sequence before submit feedback, got output:\n%s", widgetOutput)
	}
	if strings.Contains(widgetOutput, "Submitted.") {
		t.Fatalf("did not expect submitted status line in input pane, got output:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, "Session shutdown requested.") {
		t.Fatalf("expected shutdown requested status, got output:\n%s", widgetOutput)
	}
}

func TestFetchWidgetActivitySnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/activity" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(widgetActivitySnapshot{SessionID: "sess-1", State: "working", Phase: "thinking", ContextFiles: []string{"src/agentx/session.py"}})
	}))
	defer server.Close()

	snapshot, err := fetchWidgetActivitySnapshot(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchWidgetActivitySnapshot returned error: %v", err)
	}
	if snapshot.SessionID != "sess-1" {
		t.Fatalf("expected session_id sess-1, got %q", snapshot.SessionID)
	}
	if snapshot.State != "working" {
		t.Fatalf("expected state working, got %q", snapshot.State)
	}
	if snapshot.Phase != "thinking" {
		t.Fatalf("expected phase thinking, got %q", snapshot.Phase)
	}
	if len(snapshot.ContextFiles) != 1 || snapshot.ContextFiles[0] != "src/agentx/session.py" {
		t.Fatalf("expected context files in snapshot, got %#v", snapshot.ContextFiles)
	}
}

func TestWidgetActivityState_PromptLabelTransitions(t *testing.T) {
	state := newWidgetActivityState()

	if got := state.promptLabel(); got != "agentx" {
		t.Fatalf("expected default prompt label agentx, got %q", got)
	}

	state.update(widgetActivitySnapshot{State: "working", Phase: "tool"})
	if got := state.promptLabel(); got != "agentx[tool]" {
		t.Fatalf("expected working prompt label, got %q", got)
	}

	state.update(widgetActivitySnapshot{State: "completed", Phase: "none"})
	if got := state.promptLabel(); got != "agentx[done]" {
		t.Fatalf("expected done prompt label, got %q", got)
	}

	time.Sleep(1300 * time.Millisecond)
	if got := state.promptLabel(); got != "agentx" {
		t.Fatalf("expected prompt label to return to agentx, got %q", got)
	}
}

func TestWidgetActivityState_PromptLabelIncludesContextFile(t *testing.T) {
	state := newWidgetActivityState()
	state.update(widgetActivitySnapshot{State: "idle", Phase: "none", ContextFiles: []string{"src/agentx/session.py"}})
	if got := state.promptLabel(); got != "agentx[ctx:session.py]" {
		t.Fatalf("expected context label suffix, got %q", got)
	}

	state.update(widgetActivitySnapshot{State: "working", Phase: "thinking", ContextFiles: []string{"docs/ux/06_TUI_MIRROR.md"}})
	if got := state.promptLabel(); got != "agentx[thinking][ctx:06_TUI_MIRROR.md]" {
		t.Fatalf("expected working label with context suffix, got %q", got)
	}
}

func TestRunInputWidgetCommand_ClearDoesNotPrintClearedLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}

		response := "ok"
		if req.Prompt == ":clear" {
			response = "cleared"
		}
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":clear\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	widgetOutput := output.String()
	if strings.Contains(widgetOutput, "Cleared.") {
		t.Fatalf("did not expect cleared status line in input pane, got output:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, "\x1b[1A\x1b[2K\r") {
		t.Fatalf("expected input clear escape sequence for submitted commands, got output:\n%s", widgetOutput)
	}
}

func TestRunInputWidgetCommand_HelpIsHandledLocally(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		requests++

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":help\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one submit request (:q only), got %d", requests)
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "Available commands:") {
		t.Fatalf("expected local help text in output, got:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, ":context-add <path>") {
		t.Fatalf("expected context-add command discoverability in help text, got:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, ":multiline") {
		t.Fatalf("expected multiline command discoverability in help text, got:\n%s", widgetOutput)
	}
}

func TestRunInputWidgetCommand_MultilineSubmit(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":multiline\nhello\nworld\n:submit\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected two submit requests, got %d (%#v)", len(prompts), prompts)
	}
	if prompts[0] != "hello\nworld" {
		t.Fatalf("expected multiline prompt payload, got %q", prompts[0])
	}
	if prompts[1] != ":q" {
		t.Fatalf("expected :q as second prompt, got %q", prompts[1])
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "Multiline mode enabled.") {
		t.Fatalf("expected multiline mode enable notice, got:\n%s", widgetOutput)
	}
}

func TestRunInputWidgetCommand_MultilineCancelSkipsSubmit(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":ml\nline to discard\n:cancel\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected only one submit request (:q), got %d (%#v)", len(prompts), prompts)
	}
	if prompts[0] != ":q" {
		t.Fatalf("expected :q as only submitted prompt, got %q", prompts[0])
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "Multiline input cancelled.") {
		t.Fatalf("expected multiline cancel notice, got:\n%s", widgetOutput)
	}
}

func TestRunInputWidgetCommand_MultilineSendAliasAndDiscardAlias(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":multiline\nfirst\nsecond\n:send\n:ml\nthrowaway\n:discard\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected two submit requests, got %d (%#v)", len(prompts), prompts)
	}
	if prompts[0] != "first\nsecond" {
		t.Fatalf("expected multiline prompt payload from :send, got %q", prompts[0])
	}
	if prompts[1] != ":q" {
		t.Fatalf("expected :q as second prompt, got %q", prompts[1])
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "Multiline input cancelled.") {
		t.Fatalf("expected multiline cancel notice for :discard, got:\n%s", widgetOutput)
	}
}

func TestRunInputWidgetCommand_ContextAddAliasStillSubmits(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":ctx-add docs/ux/03_PANEL_DETAILS.md\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected two submit requests, got %d (%#v)", len(prompts), prompts)
	}
	if prompts[0] != ":ctx-add docs/ux/03_PANEL_DETAILS.md" {
		t.Fatalf("expected alias command to pass through unchanged, got %q", prompts[0])
	}
	if prompts[1] != ":q" {
		t.Fatalf("expected :q as second prompt, got %q", prompts[1])
	}
}

func TestInputWidgetComposeState_EscTogglesFocusWithoutSeedingControlPrompt(t *testing.T) {
	state := newInputWidgetComposeState()
	if state.focus != inputWidgetFocusInput {
		t.Fatalf("expected default focus on input, got %v", state.focus)
	}

	_ = state.handleKey(inputWidgetKey{kind: "esc"})
	if state.focus != inputWidgetFocusControl {
		t.Fatalf("expected focus to move to control after esc, got %v", state.focus)
	}
	if got := string(state.control); got != "" {
		t.Fatalf("expected control prompt to remain empty on focus, got %q", got)
	}

	_ = state.handleKey(inputWidgetKey{kind: "esc"})
	if state.focus != inputWidgetFocusInput {
		t.Fatalf("expected focus to move back to input after esc, got %v", state.focus)
	}
}

func TestInputWidgetComposeState_ArrowMovesCursorShiftArrowMovesViewport(t *testing.T) {
	state := newInputWidgetComposeState()
	state.viewportRows = 2
	state.viewportCols = 4
	state.inputLines = [][]rune{[]rune("0123456789")}
	state.cursorRow = 0
	state.cursorCol = 8
	state.ensureCursorVisible()

	priorViewCol := state.viewCol
	_ = state.handleKey(inputWidgetKey{kind: "left"})
	if state.cursorCol != 7 {
		t.Fatalf("expected left arrow to move cursor, got cursor col %d", state.cursorCol)
	}
	if state.viewCol != priorViewCol {
		t.Fatalf("expected left arrow to keep viewport offset, got viewCol=%d want=%d", state.viewCol, priorViewCol)
	}

	_ = state.handleKey(inputWidgetKey{kind: "shift_left"})
	if state.cursorCol != 7 {
		t.Fatalf("expected shift-left to keep cursor col unchanged, got %d", state.cursorCol)
	}
	if state.viewCol != priorViewCol-1 {
		t.Fatalf("expected shift-left to pan viewport left, got %d want %d", state.viewCol, priorViewCol-1)
	}
}

func TestInputWidgetComposeState_ControlEnterCommands(t *testing.T) {
	state := newInputWidgetComposeState()
	_ = state.handleKey(inputWidgetKey{kind: "esc"})

	state.control = []rune(":?")
	state.controlCursor = len(state.control)
	action := state.handleKey(inputWidgetKey{kind: "enter"})
	if action.submitPrompt != "" {
		t.Fatalf("expected :? to be local-only command, got submit prompt %q", action.submitPrompt)
	}
	if !state.showHelp {
		t.Fatalf("expected :? to toggle help on")
	}

	_ = state.handleKey(inputWidgetKey{kind: "esc"})
	state.control = []rune(":q")
	state.controlCursor = len(state.control)
	action = state.handleKey(inputWidgetKey{kind: "enter"})
	if action.submitPrompt != ":q" {
		t.Fatalf("expected :q to submit quit command, got %q", action.submitPrompt)
	}
	if !action.quitOnSubmit {
		t.Fatalf("expected :q action to request quit on submit")
	}
}

func TestNormalizeInputWidgetEscapeSequence(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantKind string
		wantOK   bool
	}{
		{name: "up", raw: "\x1b[A", wantKind: "up", wantOK: true},
		{name: "down", raw: "\x1b[B", wantKind: "down", wantOK: true},
		{name: "right", raw: "\x1b[C", wantKind: "right", wantOK: true},
		{name: "left", raw: "\x1b[D", wantKind: "left", wantOK: true},
		{name: "shift up csi", raw: "\x1b[1;2A", wantKind: "shift_up", wantOK: true},
		{name: "shift down csi", raw: "\x1b[1;2B", wantKind: "shift_down", wantOK: true},
		{name: "shift right csi", raw: "\x1b[1;2C", wantKind: "shift_right", wantOK: true},
		{name: "shift left csi", raw: "\x1b[1;2D", wantKind: "shift_left", wantOK: true},
		{name: "shift up lowercase", raw: "\x1b[a", wantKind: "shift_up", wantOK: true},
		{name: "shift down lowercase", raw: "\x1b[b", wantKind: "shift_down", wantOK: true},
		{name: "shift right lowercase", raw: "\x1b[c", wantKind: "shift_right", wantOK: true},
		{name: "shift left lowercase", raw: "\x1b[d", wantKind: "shift_left", wantOK: true},
		{name: "unknown", raw: "\x1b[9~", wantKind: "", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotKind, gotOK := normalizeInputWidgetEscapeSequence(tc.raw)
			if gotOK != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, gotOK)
			}
			if gotKind != tc.wantKind {
				t.Fatalf("expected kind %q, got %q", tc.wantKind, gotKind)
			}
		})
	}
}
