package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type filesystemWidgetEntry struct {
	Name   string
	Path   string
	IsDir  bool
	Size   int64
	Exists bool
}

type filesystemWidgetState struct {
	baseURL      string
	projectDir   string
	homeDir      string
	currentDir   string
	history      []string
	historyIndex int
	entries      []filesystemWidgetEntry
	selected     int
	status       string
}

func runFilesystemWidgetCommand(coreHTTP string, in io.Reader, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Filesystem widget failed: missing core HTTP base URL")
		return 1
	}

	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	if projectDir == "" {
		projectDir = "."
	}
	absProjectDir, err := filepath.Abs(projectDir)
	if err == nil && strings.TrimSpace(absProjectDir) != "" {
		projectDir = absProjectDir
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = projectDir
	}

	state := &filesystemWidgetState{
		baseURL:      strings.TrimRight(baseURL, "/"),
		projectDir:   projectDir,
		homeDir:      homeDir,
		history:      []string{projectDir},
		historyIndex: 0,
		currentDir:   projectDir,
		selected:     0,
		status:       "Ready",
	}
	if err := state.refresh(); err != nil {
		state.status = fmt.Sprintf("Refresh failed: %v", err)
	}

	if err := runFilesystemWidgetLoop(context.Background(), in, out, state); err != nil {
		fmt.Fprintf(out, "Filesystem widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runFilesystemWidgetLoop(ctx context.Context, in io.Reader, out io.Writer, state *filesystemWidgetState) error {
	scanner := bufio.NewScanner(in)
	for {
		if _, err := fmt.Fprintf(out, "\033[H\033[2J%s\n", state.render()); err != nil {
			return err
		}
		if _, err := fmt.Fprint(out, "files> "); err != nil {
			return err
		}

		if !scanner.Scan() {
			if scanErr := scanner.Err(); scanErr != nil {
				return scanErr
			}
			return nil
		}

		command := normalizeFilesystemWidgetCommand(scanner.Text())
		if command == "q" || command == "quit" {
			return nil
		}
		if command == "?" || command == "help" {
			state.status = "Keys: Enter/l open dir or attach file; k/up; j/down; u up; b back; f forward; h home; r refresh; a attach; e edit; q quit"
			continue
		}

		actionErr := state.handleCommand(ctx, command)
		if actionErr != nil {
			state.status = fmt.Sprintf("Error: %v", actionErr)
		}
	}
}

func normalizeFilesystemWidgetCommand(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "enter"
	}
	switch trimmed {
	case "left":
		return "b"
	case "right":
		return "f"
	case "up":
		return "k"
	case "down":
		return "j"
	case "refresh":
		return "r"
	case "home":
		return "h"
	case "back":
		return "b"
	case "forward":
		return "f"
	case "parent", "..":
		return "u"
	case "open":
		return "enter"
	case "attach", "add":
		return "a"
	case "edit":
		return "e"
	}
	return trimmed
}

func (s *filesystemWidgetState) handleCommand(ctx context.Context, command string) error {
	switch command {
	case "k":
		s.moveSelection(-1)
		s.status = "Selection moved"
		return nil
	case "j":
		s.moveSelection(1)
		s.status = "Selection moved"
		return nil
	case "u":
		if err := s.navigateParent(); err != nil {
			return err
		}
		s.status = "Moved to parent"
		return nil
	case "b":
		if err := s.navigateBack(); err != nil {
			return err
		}
		s.status = "Moved back"
		return nil
	case "f":
		if err := s.navigateForward(); err != nil {
			return err
		}
		s.status = "Moved forward"
		return nil
	case "h":
		if err := s.navigateHome(); err != nil {
			return err
		}
		s.status = "Moved home"
		return nil
	case "r":
		if err := s.refresh(); err != nil {
			return err
		}
		s.status = "Refreshed"
		return nil
	case "enter", "l":
		return s.activateSelection(ctx)
	case "a":
		if err := s.addSelectedToContext(ctx); err != nil {
			return err
		}
		s.status = "Added selected file to context"
		return nil
	case "e":
		if err := s.editSelected(); err != nil {
			return err
		}
		s.status = "Opened selected file in editor window"
		return nil
	default:
		return fmt.Errorf("unsupported command: %s", command)
	}
}

func (s *filesystemWidgetState) render() string {
	lines := []string{
		"[FILES]",
		fmt.Sprintf("project: %s", s.projectDir),
		fmt.Sprintf("current: %s", s.currentDir),
		"keys: Enter/l open or attach, k/up, j/down, u up, b back, f forward, h home, r refresh, a attach, e edit, q quit",
		fmt.Sprintf("status: %s", strings.TrimSpace(s.status)),
		"",
	}

	if len(s.entries) == 0 {
		lines = append(lines, "(empty directory)")
		return strings.Join(lines, "\n")
	}

	for idx, entry := range s.entries {
		marker := " "
		if idx == s.selected {
			marker = ">"
		}

		kind := "F"
		size := humanFileSize(entry.Size)
		if entry.IsDir {
			kind = "D"
			size = "-"
		}
		if !entry.Exists {
			kind = "?"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %-36s %8s", marker, kind, trimSingleLine(entry.Name, 36), size))
	}

	return strings.Join(lines, "\n")
}

func (s *filesystemWidgetState) moveSelection(delta int) {
	if len(s.entries) == 0 {
		s.selected = 0
		return
	}
	next := s.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.entries) {
		next = len(s.entries) - 1
	}
	s.selected = next
}

func (s *filesystemWidgetState) refresh() error {
	entries, err := os.ReadDir(s.currentDir)
	if err != nil {
		return err
	}

	results := make([]filesystemWidgetEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := filepath.Join(s.currentDir, entry.Name())
		info, infoErr := entry.Info()
		size := int64(0)
		exists := infoErr == nil
		if infoErr == nil {
			size = info.Size()
		}

		results = append(results, filesystemWidgetEntry{
			Name:   entry.Name(),
			Path:   entryPath,
			IsDir:  entry.IsDir(),
			Size:   size,
			Exists: exists,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].IsDir != results[j].IsDir {
			return results[i].IsDir
		}
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	s.entries = results
	if len(s.entries) == 0 {
		s.selected = 0
		return nil
	}
	if s.selected >= len(s.entries) {
		s.selected = len(s.entries) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
	return nil
}

func (s *filesystemWidgetState) navigateTo(path string, addToHistory bool) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return errors.New("path cannot be empty")
	}
	info, err := os.Stat(clean)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", clean)
	}

	s.currentDir = clean
	if addToHistory {
		if s.historyIndex < len(s.history)-1 {
			s.history = append([]string{}, s.history[:s.historyIndex+1]...)
		}
		s.history = append(s.history, clean)
		s.historyIndex = len(s.history) - 1
	}

	return s.refresh()
}

func (s *filesystemWidgetState) navigateParent() error {
	parent := filepath.Dir(s.currentDir)
	if parent == s.currentDir {
		return nil
	}
	return s.navigateTo(parent, true)
}

func (s *filesystemWidgetState) navigateBack() error {
	if s.historyIndex <= 0 {
		return errors.New("already at oldest history entry")
	}
	s.historyIndex--
	s.currentDir = s.history[s.historyIndex]
	return s.refresh()
}

func (s *filesystemWidgetState) navigateForward() error {
	if s.historyIndex >= len(s.history)-1 {
		return errors.New("already at newest history entry")
	}
	s.historyIndex++
	s.currentDir = s.history[s.historyIndex]
	return s.refresh()
}

func (s *filesystemWidgetState) navigateHome() error {
	return s.navigateTo(s.homeDir, true)
}

func (s *filesystemWidgetState) selectedEntry() (filesystemWidgetEntry, error) {
	if len(s.entries) == 0 {
		return filesystemWidgetEntry{}, errors.New("no entries available")
	}
	if s.selected < 0 || s.selected >= len(s.entries) {
		return filesystemWidgetEntry{}, errors.New("invalid selection")
	}
	return s.entries[s.selected], nil
}

func (s *filesystemWidgetState) activateSelection(ctx context.Context) error {
	entry, err := s.selectedEntry()
	if err != nil {
		return err
	}
	if entry.IsDir {
		if err := s.navigateTo(entry.Path, true); err != nil {
			return err
		}
		s.status = "Entered directory"
		return nil
	}
	if err := s.addSelectedToContext(ctx); err != nil {
		return err
	}
	s.status = "Added selected file to context"
	return nil
}

func (s *filesystemWidgetState) addSelectedToContext(ctx context.Context) error {
	entry, err := s.selectedEntry()
	if err != nil {
		return err
	}
	if entry.IsDir {
		return errors.New("selection is a directory; choose a file")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return errors.New("missing core HTTP base URL")
	}

	submitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, submitErr := submitPromptToCore(submitCtx, s.baseURL, ":context-add "+entry.Path)
	if submitErr != nil {
		return submitErr
	}
	return nil
}

func (s *filesystemWidgetState) editSelected() error {
	entry, err := s.selectedEntry()
	if err != nil {
		return err
	}
	if entry.IsDir {
		return errors.New("selection is a directory; choose a file")
	}
	return launchEditorTmuxWindow(entry.Path)
}

func launchEditorTmuxWindow(filePath string) error {
	sessionName, err := resolveTmuxSessionName()
	if err != nil {
		return err
	}

	editor := resolveEditor(os.Getenv("EDITOR"))
	windowName := buildEditorWindowName(filePath)
	editorCommand := buildEditorCommand(editor, filePath)
	cmd := exec.Command("tmux", "new-window", "-t", sessionName+":", "-n", windowName, editorCommand)
	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("failed launching editor window: %w", runErr)
	}
	return nil
}

func resolveTmuxSessionName() (string, error) {
	fromEnv := strings.TrimSpace(os.Getenv("AGENTX_TMUX_SESSION"))
	if fromEnv != "" {
		return fromEnv, nil
	}

	output, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err == nil {
		sessionName := strings.TrimSpace(string(output))
		if sessionName != "" {
			return sessionName, nil
		}
	}

	sessionID := strings.TrimSpace(os.Getenv("AGENTX_SESSION_ID"))
	username := strings.TrimSpace(os.Getenv("AGENTX_USERNAME"))
	if sessionID != "" && username != "" {
		return buildTmuxSessionName(username, sessionID), nil
	}
	return "", errors.New("unable to resolve tmux session name")
}

func resolveEditor(raw string) string {
	editor := strings.TrimSpace(raw)
	if editor == "" {
		return "vim"
	}
	return editor
}

func buildEditorWindowName(filePath string) string {
	name := filepath.Base(strings.TrimSpace(filePath))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "file"
	}
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		name = "file"
	}
	return trimSingleLine("edit-"+name, 32)
}

func buildEditorCommand(editor string, filePath string) string {
	return fmt.Sprintf("%s %s", shellSingleQuote(editor), shellSingleQuote(filePath))
}

func humanFileSize(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + "B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	unitIdx := 0
	for value >= 1024 && unitIdx < len(units)-1 {
		value = value / 1024
		unitIdx++
	}
	return fmt.Sprintf("%.1f%s", value, units[unitIdx])
}
