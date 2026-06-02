package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeFilesystemWidgetCommand(t *testing.T) {
	cases := map[string]string{
		"":        "enter",
		" ":       "space",
		"   ":     "space",
		"left":    "b",
		"RIGHT":   "f",
		"up":      "k",
		"down":    "j",
		"refresh": "r",
		"home":    "h",
		"pageup":  "pgup",
		"pgdn":    "pgdn",
		"top":     "top",
		"end":     "end",
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
		projectDir:   "/tmp/project",
		currentDir:   "/tmp/project",
		viewportRows: 12,
		status:       "Ready",
	}

	rendered := state.render()
	if !strings.Contains(rendered, "a attach, e edit") {
		t.Fatalf("expected render key legend to include attach/edit actions, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "space/s toggle (soft)") {
		t.Fatalf("expected render key legend to include soft select action, got:\n%s", rendered)
	}
}

func TestFilesystemWidgetRender_OverflowOrientationAndSelectionVisibility(t *testing.T) {
	entries := make([]filesystemWidgetEntry, 0, 8)
	for i := 0; i < 8; i++ {
		entries = append(entries, filesystemWidgetEntry{
			Name:   "item-" + strconv.Itoa(i),
			Path:   filepath.Join("/tmp", "item-"+strconv.Itoa(i)),
			Exists: true,
		})
	}

	state := &filesystemWidgetState{
		projectDir:   "/tmp/project",
		currentDir:   "/tmp/project",
		entries:      entries,
		selected:     0,
		viewOffset:   0,
		viewportRows: 3,
		status:       "Ready",
	}

	state.moveSelectionTo(6)
	rendered := state.render()

	if !strings.Contains(rendered, "showing 5-7 of 8") {
		t.Fatalf("expected deterministic overflow orientation, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, ">[ ] [F] item-6") {
		t.Fatalf("expected selected row to remain visible in rendered viewport, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "item-0") {
		t.Fatalf("expected rows outside viewport to be hidden in overflow render, got:\n%s", rendered)
	}
}

func TestFilesystemWidgetHandleCommand_PageNavigation(t *testing.T) {
	entries := make([]filesystemWidgetEntry, 0, 20)
	for i := 0; i < 20; i++ {
		entries = append(entries, filesystemWidgetEntry{Name: "f" + strconv.Itoa(i), Path: "/tmp/f" + strconv.Itoa(i), Exists: true})
	}

	state := &filesystemWidgetState{entries: entries, viewportRows: 5}

	if err := state.handleCommand(context.Background(), "pgdn"); err != nil {
		t.Fatalf("pgdn returned error: %v", err)
	}
	if state.selected != 4 {
		t.Fatalf("expected pgdn to move selection by page, got %d", state.selected)
	}

	if err := state.handleCommand(context.Background(), "end"); err != nil {
		t.Fatalf("end returned error: %v", err)
	}
	if state.selected != 19 {
		t.Fatalf("expected end to move to last row, got %d", state.selected)
	}

	if err := state.handleCommand(context.Background(), "top"); err != nil {
		t.Fatalf("top returned error: %v", err)
	}
	if state.selected != 0 {
		t.Fatalf("expected top to move to first row, got %d", state.selected)
	}
}

func TestFilesystemWidgetHandleCommand_SoftSelectToggleVisibleInRender(t *testing.T) {
	state := &filesystemWidgetState{
		projectDir:   "/tmp/project",
		currentDir:   "/tmp/project",
		entries:      []filesystemWidgetEntry{{Name: "file.txt", Path: "/tmp/file.txt", Exists: true}},
		selected:     0,
		viewportRows: 5,
		softSelected: map[string]bool{},
	}

	if err := state.handleCommand(context.Background(), "space"); err != nil {
		t.Fatalf("space returned error: %v", err)
	}
	rendered := state.render()
	if !strings.Contains(rendered, ">[x] [F] file.txt") {
		t.Fatalf("expected soft-selected state marker in render, got:\n%s", rendered)
	}

	if err := state.handleCommand(context.Background(), "space"); err != nil {
		t.Fatalf("second space returned error: %v", err)
	}
	rendered = state.render()
	if !strings.Contains(rendered, ">[ ] [F] file.txt") {
		t.Fatalf("expected soft-selected marker to clear after second toggle, got:\n%s", rendered)
	}
}

func TestFilesystemWidgetHandleCommand_ReturnHardSelectActivates(t *testing.T) {
	projectDir := t.TempDir()
	filePath := filepath.Join(projectDir, "note.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed creating file: %v", err)
	}

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
		_ = json.NewEncoder(w).Encode(submitResponse{Response: "ok"})
	}))
	defer server.Close()

	state := &filesystemWidgetState{
		baseURL:      strings.TrimRight(server.URL, "/"),
		projectDir:   projectDir,
		currentDir:   projectDir,
		entries:      []filesystemWidgetEntry{{Name: "note.txt", Path: filePath, Exists: true}},
		selected:     0,
		viewportRows: 5,
		softSelected: map[string]bool{filepath.Join(projectDir, "other.txt"): true},
	}

	if err := state.handleCommand(context.Background(), "enter"); err != nil {
		t.Fatalf("enter returned error: %v", err)
	}
	if got, want := recordedPrompt, ":context-add "+filePath; got != want {
		t.Fatalf("expected hard-select enter to trigger primary action prompt %q, got %q", want, got)
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

	count, err := state.addSelectedToContext(context.Background())
	if err != nil {
		t.Fatalf("addSelectedToContext returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one file to be added, got %d", count)
	}

	expected := ":context-add " + filePath
	if recordedPrompt != expected {
		t.Fatalf("expected prompt %q, got %q", expected, recordedPrompt)
	}
}

func TestFilesystemWidgetAddSelectedToContext_UsesSoftSelectedSetInViewOrder(t *testing.T) {
	var prompts []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed decoding submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)
		_ = json.NewEncoder(w).Encode(submitResponse{Response: "context-added"})
	}))
	defer server.Close()

	entryA := filesystemWidgetEntry{Name: "a.txt", Path: "/tmp/a.txt", Exists: true}
	entryB := filesystemWidgetEntry{Name: "b.txt", Path: "/tmp/b.txt", Exists: true}
	entryC := filesystemWidgetEntry{Name: "c.txt", Path: "/tmp/c.txt", Exists: true}

	state := &filesystemWidgetState{
		baseURL: strings.TrimRight(server.URL, "/"),
		entries: []filesystemWidgetEntry{entryA, entryB, entryC},
		selected: 1,
		softSelected: map[string]bool{
			entryA.Path: true,
			entryC.Path: true,
		},
	}

	count, err := state.addSelectedToContext(context.Background())
	if err != nil {
		t.Fatalf("addSelectedToContext returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two files to be added, got %d", count)
	}

	joined := strings.Join(prompts, "|")
	if got, want := joined, ":context-add /tmp/a.txt|:context-add /tmp/c.txt"; got != want {
		t.Fatalf("expected deterministic soft-selected prompt order %q, got %q", want, got)
	}
}

func TestFilesystemWidgetAddSelectedToContext_SoftSelectedDirectoriesOnlyReturnsError(t *testing.T) {
	state := &filesystemWidgetState{
		baseURL: "http://127.0.0.1:65535",
		entries: []filesystemWidgetEntry{{Name: "docs", Path: "/tmp/docs", IsDir: true, Exists: true}},
		selected: 0,
		softSelected: map[string]bool{
			"/tmp/docs": true,
		},
	}

	_, err := state.addSelectedToContext(context.Background())
	if err == nil {
		t.Fatal("expected error for directory-only soft-selected set")
	}
	if !strings.Contains(err.Error(), "soft-selected entries are directories") {
		t.Fatalf("expected directory-set compatibility error, got %v", err)
	}
}

func TestFilesystemWidgetHandleCommand_AttachUsesSoftSelectedSetStatus(t *testing.T) {
	var prompts []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed decoding submit request: %v", err)
		}
		prompts = append(prompts, req.Prompt)
		_ = json.NewEncoder(w).Encode(submitResponse{Response: "ok"})
	}))
	defer server.Close()

	state := &filesystemWidgetState{
		baseURL: strings.TrimRight(server.URL, "/"),
		entries: []filesystemWidgetEntry{
			{Name: "a.txt", Path: "/tmp/a.txt", Exists: true},
			{Name: "b.txt", Path: "/tmp/b.txt", Exists: true},
		},
		selected: 0,
		softSelected: map[string]bool{
			"/tmp/a.txt": true,
			"/tmp/b.txt": true,
		},
	}

	if err := state.handleCommand(context.Background(), "a"); err != nil {
		t.Fatalf("attach command returned error: %v", err)
	}
	if state.status != "Added 2 selected files to context" {
		t.Fatalf("expected multi-select attach status, got %q", state.status)
	}
	if got, want := strings.Join(prompts, "|"), ":context-add /tmp/a.txt|:context-add /tmp/b.txt"; got != want {
		t.Fatalf("expected attach command to use soft-selected set in view order, got %q", got)
	}
}

func TestFilesystemWidgetHandleCommand_EditLaunchesEditorWindow(t *testing.T) {
	logPath := setupFakeTmux(t)
	t.Setenv("AGENTX_TMUX_SESSION", "sess-files")
	t.Setenv("EDITOR", "nvim")

	filePath := filepath.Join(t.TempDir(), "notes file.txt")
	state := &filesystemWidgetState{
		entries:  []filesystemWidgetEntry{{Name: filepath.Base(filePath), Path: filePath, IsDir: false, Exists: true}},
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

func TestFilesystemWidgetHandleCommand_EditUsesSoftSelectedSetInViewOrder(t *testing.T) {
	logPath := setupFakeTmux(t)
	t.Setenv("AGENTX_TMUX_SESSION", "sess-files")
	t.Setenv("EDITOR", "nvim")

	root := t.TempDir()
	fileA := filepath.Join(root, "a one.txt")
	fileB := filepath.Join(root, "b two.txt")

	state := &filesystemWidgetState{
		entries: []filesystemWidgetEntry{
			{Name: filepath.Base(fileA), Path: fileA, IsDir: false, Exists: true},
			{Name: filepath.Base(fileB), Path: fileB, IsDir: false, Exists: true},
		},
		selected: 0,
		softSelected: map[string]bool{
			fileA: true,
			fileB: true,
		},
	}

	if err := state.handleCommand(context.Background(), "e"); err != nil {
		t.Fatalf("handleCommand returned error: %v", err)
	}
	if state.status != "Opened 2 selected files in editor windows" {
		t.Fatalf("expected multi-edit status update, got %q", state.status)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "'nvim' '"+fileA+"'") {
		t.Fatalf("expected first soft-selected file launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "'nvim' '"+fileB+"'") {
		t.Fatalf("expected second soft-selected file launch command, got:\n%s", commands)
	}
	firstIdx := strings.Index(commands, "'nvim' '"+fileA+"'")
	secondIdx := strings.Index(commands, "'nvim' '"+fileB+"'")
	if firstIdx == -1 || secondIdx == -1 || firstIdx >= secondIdx {
		t.Fatalf("expected deterministic view-order editor launches, got:\n%s", commands)
	}
}

func TestFilesystemWidgetHandleCommand_EditSoftSelectedDirectoriesOnlyReturnsError(t *testing.T) {
	state := &filesystemWidgetState{
		entries: []filesystemWidgetEntry{{Name: "docs", Path: "/tmp/docs", IsDir: true, Exists: true}},
		selected: 0,
		softSelected: map[string]bool{
			"/tmp/docs": true,
		},
	}

	err := state.handleCommand(context.Background(), "e")
	if err == nil {
		t.Fatal("expected error for directory-only soft-selected edit set")
	}
	if !strings.Contains(err.Error(), "soft-selected entries are directories") {
		t.Fatalf("expected directory-set compatibility error, got %v", err)
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

func TestResolveFilesystemViewportRows_DefaultAndInvalidFallback(t *testing.T) {
	t.Setenv("AGENTX_FILES_VIEWPORT_ROWS", "")
	if got := resolveFilesystemViewportRows(); got != defaultFilesystemViewportRows {
		t.Fatalf("expected default rows %d, got %d", defaultFilesystemViewportRows, got)
	}

	t.Setenv("AGENTX_FILES_VIEWPORT_ROWS", "abc")
	if got := resolveFilesystemViewportRows(); got != defaultFilesystemViewportRows {
		t.Fatalf("expected invalid value fallback to default rows %d, got %d", defaultFilesystemViewportRows, got)
	}

	t.Setenv("AGENTX_FILES_VIEWPORT_ROWS", "7")
	if got := resolveFilesystemViewportRows(); got != 7 {
		t.Fatalf("expected explicit rows 7, got %d", got)
	}
}
