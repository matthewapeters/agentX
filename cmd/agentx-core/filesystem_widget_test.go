package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFilesystemWidgetCommand(t *testing.T) {
	cases := map[string]string{
		"":        "enter",
		"   ":     "enter",
		"left":    "b",
		"RIGHT":   "f",
		"up":      "k",
		"down":    "j",
		"refresh": "r",
		"home":    "h",
		"parent":  "u",
		"open":    "enter",
		"attach":  "a",
		"edit":    "e",
	}
	for input, expected := range cases {
		if got := normalizeFilesystemWidgetCommand(input); got != expected {
			t.Fatalf("normalize command mismatch for %q: got %q want %q", input, got, expected)
		}
	}
}

func TestFilesystemWidgetActivateSelection_DirectoryNavigates(t *testing.T) {
	projectDir := t.TempDir()
	subDir := filepath.Join(projectDir, "docs")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed creating subdir: %v", err)
	}

	state := &filesystemWidgetState{
		baseURL:      "http://127.0.0.1:65535",
		projectDir:   projectDir,
		homeDir:      projectDir,
		currentDir:   projectDir,
		history:      []string{projectDir},
		historyIndex: 0,
		entries: []filesystemWidgetEntry{
			{Name: "docs", Path: subDir, IsDir: true, Exists: true},
		},
		selected: 0,
	}

	if err := state.activateSelection(context.Background()); err != nil {
		t.Fatalf("activateSelection returned error: %v", err)
	}
	if state.currentDir != subDir {
		t.Fatalf("expected directory navigation to %q, got %q", subDir, state.currentDir)
	}
	if len(state.history) != 2 || state.history[1] != subDir {
		t.Fatalf("expected navigation history update, got %#v", state.history)
	}
}

func TestFilesystemWidgetRender_IncludesAttachAndEditActions(t *testing.T) {
	state := &filesystemWidgetState{
		projectDir: "/tmp/project",
		currentDir: "/tmp/project",
		status:     "Ready",
	}

	rendered := state.render()
	if !strings.Contains(rendered, "a attach, e edit") {
		t.Fatalf("expected render key legend to include attach/edit actions, got:\n%s", rendered)
	}
}

func TestFilesystemWidgetAddSelectedToContext_UsesSubmitEndpoint(t *testing.T) {
	filePath := filepath.Join("src", "agentx", "session.py")
	var recordedPrompt string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed decoding submit request: %v", err)
		}
		recordedPrompt = req.Prompt
		_ = json.NewEncoder(w).Encode(submitResponse{Response: "context-added"})
	}))
	defer server.Close()

	state := &filesystemWidgetState{
		baseURL: strings.TrimRight(server.URL, "/"),
		entries: []filesystemWidgetEntry{{Name: "session.py", Path: filePath, IsDir: false, Exists: true}},
	}

	if err := state.addSelectedToContext(context.Background()); err != nil {
		t.Fatalf("addSelectedToContext returned error: %v", err)
	}

	expected := ":context-add " + filePath
	if recordedPrompt != expected {
		t.Fatalf("expected prompt %q, got %q", expected, recordedPrompt)
	}
}

func TestFilesystemWidgetHandleCommand_EditLaunchesEditorWindow(t *testing.T) {
	logPath := setupFakeTmux(t)
	t.Setenv("AGENTX_TMUX_SESSION", "sess-files")
	t.Setenv("EDITOR", "nvim")

	filePath := filepath.Join(t.TempDir(), "notes file.txt")
	state := &filesystemWidgetState{
		entries: []filesystemWidgetEntry{{Name: filepath.Base(filePath), Path: filePath, IsDir: false, Exists: true}},
		selected: 0,
	}

	if err := state.handleCommand(context.Background(), "e"); err != nil {
		t.Fatalf("handleCommand returned error: %v", err)
	}
	if state.status != "Opened selected file in editor window" {
		t.Fatalf("expected edit status update, got %q", state.status)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "new-window -t sess-files:") {
		t.Fatalf("expected tmux new-window command for editor launch, got:\n%s", commands)
	}
	if !strings.Contains(commands, "'nvim' '"+filePath+"'") {
		t.Fatalf("expected quoted editor launch command, got:\n%s", commands)
	}
}

func TestResolveEditorFallsBackToVim(t *testing.T) {
	if got := resolveEditor("   "); got != "vim" {
		t.Fatalf("expected vim fallback, got %q", got)
	}
	if got := resolveEditor("nvim"); got != "nvim" {
		t.Fatalf("expected explicit editor passthrough, got %q", got)
	}
}

func TestBuildEditorCommandQuotesArguments(t *testing.T) {
	command := buildEditorCommand("vim", "/tmp/a b.txt")
	if !strings.Contains(command, "'vim'") {
		t.Fatalf("expected quoted editor, got %q", command)
	}
	if !strings.Contains(command, "'/tmp/a b.txt'") {
		t.Fatalf("expected quoted file path, got %q", command)
	}
}

func TestBuildEditorWindowNameSanitizes(t *testing.T) {
	name := buildEditorWindowName("/tmp/a b+c.txt")
	if !strings.HasPrefix(name, "edit-") {
		t.Fatalf("expected edit- prefix, got %q", name)
	}
	if strings.ContainsAny(name, " +/") {
		t.Fatalf("expected sanitized window name, got %q", name)
	}
}

func TestHumanFileSize(t *testing.T) {
	if got := humanFileSize(128); got != "128B" {
		t.Fatalf("unexpected bytes representation: %q", got)
	}
	if got := humanFileSize(1024); !strings.Contains(got, "KB") {
		t.Fatalf("expected KB representation, got %q", got)
	}
}
