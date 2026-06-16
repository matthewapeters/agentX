package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	viewOffset   int
	viewportRows int
	viewportCols int
	showHelp     bool
	softSelected map[string]bool
	status       string
	bellPending  bool

	startupViewportApplied bool
}

const defaultFilesystemViewportRows = 12
const defaultFilesystemViewportCols = 58
const filesystemWidgetBorderLines = 2

const (
	filesystemParentRowStyle  = "\033[48;5;238m\033[97m"
	filesystemHiddenFileStyle = ansiMagenta
	filesystemConfigFileStyle = ansiYellow
	filesystemGoFileStyle     = ansiCyan
	filesystemPythonFileStyle = ansiBlue
	filesystemJSFileStyle     = ansiYellow
	filesystemCFileStyle      = ansiGreen
	filesystemOtherCodeStyle  = ansiGreen
	filesystemMissingStyle    = ansiRed
)

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
		viewOffset:   0,
		viewportRows: resolveFilesystemViewportRows(),
		softSelected: map[string]bool{},
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
	commandReader, promptMode, cleanup := newFilesystemWidgetCommandReader(in)
	defer cleanup()
	hideTerminalCursor(out)
	defer showTerminalCursor(out)
	startupHeight, startupWidth := resolveWidgetPaneSizeAtStartup(out)
	state.seedViewportFromStartup(startupHeight, startupWidth, promptMode)
	var previousLines []string
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	type commandEvent struct {
		command string
		err     error
	}
	commandEvents := make(chan commandEvent, 16)
	go func() {
		defer close(commandEvents)
		for {
			command, readErr := commandReader()
			if readErr != nil {
				select {
				case commandEvents <- commandEvent{err: readErr}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case commandEvents <- commandEvent{command: command}:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		state.adaptViewportToTerminal(out, promptMode)
		currentLines := filesystemWidgetFrameLines(state.render())
		renderChanged := len(previousLines) == 0 || strings.Join(previousLines, "\n") != strings.Join(currentLines, "\n")
		if renderChanged {
			if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
				return err
			}
			previousLines = currentLines
			if promptMode {
				if _, err := fmt.Fprint(out, "files> "); err != nil {
					return err
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-commandEvents:
			if !ok {
				return nil
			}
			if event.err != nil {
				if errors.Is(event.err, io.EOF) {
					return nil
				}
				return event.err
			}
			command := normalizeFilesystemWidgetControlCommand(event.command)

			action := handleWidgetLoopControlCommand(command, widgetLoopControlHandlers{
				QuitTokens: []string{"q", "quit"},
				HelpTokens: []string{"help"},
				OnHelp: func() {
					state.toggleHelp()
				},
			})
			if action == widgetLoopControlQuit {
				return nil
			}
			if action == widgetLoopControlHandled {
				continue
			}

			actionErr := state.handleCommand(ctx, command)
			if actionErr != nil {
				state.status = fmt.Sprintf("Error: %v", actionErr)
				continue
			}
			if state.consumeBell() {
				if _, err := fmt.Fprint(out, "\a"); err != nil {
					return err
				}
			}
		case <-ticker.C:
		}
	}
}

func hideTerminalCursor(out io.Writer) {
	_, _ = fmt.Fprint(out, "\033[?25l")
}

func showTerminalCursor(out io.Writer) {
	_, _ = fmt.Fprint(out, "\033[?25h")
}

func (s *filesystemWidgetState) adaptViewportToTerminal(out io.Writer, promptMode bool) {
	if s.startupViewportApplied {
		s.startupViewportApplied = false
		return
	}
	rows, cols := resolveWidgetViewport(out, s.headerLineCount(), filesystemWidgetBorderLines, promptMode, defaultFilesystemViewportCols, 1)
	s.viewportRows = rows
	s.viewportCols = cols
}

func (s *filesystemWidgetState) seedViewportFromStartup(height int, width int, promptMode bool) {
	s.applyViewportDimensions(height, width, promptMode)
	s.startupViewportApplied = true
}

func (s *filesystemWidgetState) applyViewportDimensions(height int, width int, promptMode bool) {
	rows := height - s.headerLineCount() - filesystemWidgetBorderLines
	if promptMode {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	contentCols := defaultFilesystemViewportCols
	if width > 4 {
		contentCols = width - 4
	} else {
		contentCols = 1
	}
	s.viewportRows = rows
	s.viewportCols = contentCols
}

func (s *filesystemWidgetState) headerLineCount() int {
	count := 7
	if s.showHelp {
		count += 2
	}
	return count
}

func (s *filesystemWidgetState) toggleHelp() {
	s.showHelp = !s.showHelp
	if s.showHelp {
		s.status = "Help shown"
		return
	}
	s.status = "Help hidden"
}

func (s *filesystemWidgetState) viewportContentWidth() int {
	if s.viewportCols > 0 {
		return s.viewportCols
	}
	return defaultFilesystemViewportCols
}

func (s *filesystemWidgetState) clipToViewport(line string) string {
	if s.viewportCols <= 0 {
		return line
	}
	return trimSingleLine(line, s.viewportContentWidth()+4)
}

func writeFilesystemWidgetFrame(out io.Writer, body string) error {
	return writeFilesystemWidgetFrameDiff(out, nil, filesystemWidgetFrameLines(body))
}

func filesystemWidgetFrameLines(body string) []string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func writeFilesystemWidgetFrameDiff(out io.Writer, previous []string, current []string) error {
	if len(previous) == 0 {
		return writeFilesystemWidgetFullFrame(out, current)
	}

	maxLines := len(current)
	if len(previous) > maxLines {
		maxLines = len(previous)
	}
	for idx := 0; idx < maxLines; idx++ {
		prevLine := ""
		if idx < len(previous) {
			prevLine = previous[idx]
		}
		currLine := ""
		if idx < len(current) {
			currLine = current[idx]
		}
		if prevLine == currLine {
			continue
		}
		if _, err := fmt.Fprintf(out, "\033[%d;1H\033[2K%s", idx+1, normalizeFilesystemWidgetLine(currLine)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "\033[%d;1H", len(current)+1)
	return err
}

func writeFilesystemWidgetFullFrame(out io.Writer, lines []string) error {
	if _, err := fmt.Fprint(out, "\033[H\033[2J"); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(out, "%s\r\n", normalizeFilesystemWidgetLine(line)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "\033[%d;1H", len(lines)+1)
	return err
}

func normalizeFilesystemWidgetLine(line string) string {
	normalized := strings.ReplaceAll(line, "\r", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	return normalized
}

func newFilesystemWidgetCommandReader(in io.Reader) (func() (string, error), bool, func()) {
	return newWidgetCommandReader(in, normalizeFilesystemWidgetCommand)
}

func normalizeFilesystemWidgetControlCommand(raw string) string {
	return normalizeWidgetControlCommand(raw, defaultWidgetControlAliases())
}

func normalizeFilesystemWidgetCommand(raw string) string {
	return normalizeWidgetCommand(raw)
}

func normalizeFilesystemWidgetEscapeSequence(raw string) (string, bool) {
	return normalizeWidgetEscapeSequence(raw)
}

func (s *filesystemWidgetState) handleCommand(ctx context.Context, command string) error {
	switch command {
	case "k":
		if !s.moveSelection(-1) {
			s.status = "Top of list"
			s.requestBell()
		}
		return nil
	case "j":
		if !s.moveSelection(1) {
			s.status = "Bottom of list"
			s.requestBell()
		}
		return nil
	case "pgup":
		if !s.moveSelectionPage(-1) {
			s.status = "Top of list"
			s.requestBell()
		}
		return nil
	case "pgdn":
		if !s.moveSelectionPage(1) {
			s.status = "Bottom of list"
			s.requestBell()
		}
		return nil
	case "top":
		if !s.moveSelectionTo(0) {
			s.status = "Top of list"
			s.requestBell()
		}
		return nil
	case "end":
		if !s.moveSelectionTo(len(s.entries) - 1) {
			s.status = "Bottom of list"
			s.requestBell()
		}
		return nil
	case "space", "s", "toggle":
		if err := s.toggleSoftSelection(); err != nil {
			return err
		}
		s.status = "Selection toggled"
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
		addedCount, err := s.addSelectedToContext(ctx)
		if err != nil {
			return err
		}
		if addedCount == 1 {
			s.status = "Added selected file to context"
		} else {
			s.status = fmt.Sprintf("Added %d selected files to context", addedCount)
		}
		return nil
	case "e":
		editedCount, err := s.editSelected()
		if err != nil {
			return err
		}
		if editedCount == 1 {
			s.status = "Opened selected file in editor window"
		} else {
			s.status = fmt.Sprintf("Opened %d selected files in editor windows", editedCount)
		}
		return nil
	default:
		return fmt.Errorf("unsupported command: %s", command)
	}
}

func (s *filesystemWidgetState) render() string {
	s.ensureSelectionVisible()
	start, end := s.visibleRange()
	total := len(s.entries)
	contentWidth := s.viewportContentWidth()
	horizontalBorder := strings.Repeat("─", contentWidth+2)
	showFrom := 0
	showTo := 0
	if total > 0 {
		showFrom = start + 1
		showTo = end
	}

	lines := []string{
		"[FILES]",
		s.clipToViewport(fmt.Sprintf("project: %s", s.projectDir)),
		s.clipToViewport(fmt.Sprintf("current: %s", s.currentDir)),
		s.clipToViewport("help: ? toggle"),
		s.clipToViewport(fmt.Sprintf("showing %d-%d of %d", showFrom, showTo, total)),
		s.clipToViewport(fmt.Sprintf("status: %s", strings.TrimSpace(s.status))),
		"",
	}
	if s.showHelp {
		lines = append(lines,
			s.clipToViewport("keys: Enter open, Space toggle, Up/Down move, PageUp/PageDown page, Home/End jump"),
			s.clipToViewport("nav: .. or u parent, b back, f forward, h home-dir, r refresh, a attach, e edit, q quit"),
		)
	}

	if total == 0 {
		lines = append(lines, "┌"+horizontalBorder+"┐")
		lines = append(lines, fmt.Sprintf("│ %-*s │", contentWidth, trimSingleLine("(empty directory)", contentWidth)))
		lines = append(lines, "└"+horizontalBorder+"┘")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "┌"+horizontalBorder+"┐")

	for idx := start; idx < end; idx++ {
		entry := s.entries[idx]
		row := s.formatEntryRow(entry, idx == s.selected)
		lines = append(lines, fmt.Sprintf("│ %s │", padVisibleWidth(row, contentWidth)))
	}
	lines = append(lines, "└"+horizontalBorder+"┘")

	return strings.Join(lines, "\n")
}

func (s *filesystemWidgetState) formatEntryRow(entry filesystemWidgetEntry, selected bool) string {
	marker := " "
	if selected {
		marker = ">"
	}
	soft := "[ ]"
	if s.isSoftSelected(entry) {
		soft = "[x]"
	}

	size := humanFileSize(entry.Size)
	if entry.IsDir {
		size = "-"
	}

	prefix := fmt.Sprintf("%s %s ", marker, soft)
	const minNameWidth = 6
	sizeWidth := 8
	contentWidth := s.viewportContentWidth()
	icon := filesystemEntryIcon(entry)
	iconWidth := len([]rune(icon)) + 1
	nameWidth := contentWidth - len([]rune(prefix)) - iconWidth - 1 - sizeWidth
	if nameWidth < minNameWidth {
		nameWidth = minNameWidth
		sizeWidth = contentWidth - len([]rune(prefix)) - iconWidth - 1 - nameWidth
		if sizeWidth < 1 {
			sizeWidth = 1
		}
	}

	name := trimSingleLine(entry.Name, nameWidth)
	name = fmt.Sprintf("%-*s", nameWidth, name)
	size = trimSingleLine(size, sizeWidth)

	iconAndName := fmt.Sprintf("%s %s", icon, name)
	styledIconAndName := styleToken(iconAndName, filesystemEntryStyle(entry))
	if entry.Name == ".." {
		styledIconAndName = filesystemParentRowStyle + iconAndName + ansiReset
	}

	return fmt.Sprintf("%s%s %*s", prefix, styledIconAndName, sizeWidth, size)
}

func filesystemEntryIcon(entry filesystemWidgetEntry) string {
	if entry.Name == ".." {
		return "⤴"
	}
	if entry.IsDir {
		return "📁"
	}
	if !entry.Exists {
		return "❓"
	}
	return "📄"
}

func filesystemEntryStyle(entry filesystemWidgetEntry) string {
	if !entry.Exists {
		return filesystemMissingStyle
	}
	if entry.IsDir {
		return ansiReverse
	}
	if strings.HasPrefix(entry.Name, ".") {
		return filesystemHiddenFileStyle
	}
	if isFilesystemConfigFile(entry.Name) {
		return filesystemConfigFileStyle
	}
	if style, ok := filesystemCodeFileStyle(entry.Name); ok {
		return style
	}
	return ansiReset
}

func isFilesystemConfigFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".ini", ".toml", ".yaml", ".yml", ".json", ".xml", ".conf", ".cfg":
		return true
	default:
		return false
	}
}

func filesystemCodeFileStyle(name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return filesystemGoFileStyle, true
	case ".py":
		return filesystemPythonFileStyle, true
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx":
		return filesystemJSFileStyle, true
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return filesystemCFileStyle, true
	case ".java", ".kt", ".rs", ".rb", ".php", ".cs", ".swift", ".sh", ".bash", ".zsh":
		return filesystemOtherCodeStyle, true
	default:
		return "", false
	}
}

func trimSingleLinePreserveSpacing(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	singleLine := strings.ReplaceAll(value, "\r", " ")
	singleLine = strings.ReplaceAll(singleLine, "\n", " ")
	singleLine = strings.ReplaceAll(singleLine, "\t", " ")
	if singleLine == "" {
		return ""
	}
	if len(singleLine) <= limit {
		return singleLine
	}
	if limit < 4 {
		return singleLine[:limit]
	}
	return strings.TrimRight(singleLine[:limit-3], " ") + "..."
}

func (s *filesystemWidgetState) moveSelection(delta int) bool {
	if len(s.entries) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	oldSelected := s.selected
	oldOffset := s.viewOffset
	next := s.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.entries) {
		next = len(s.entries) - 1
	}
	s.selected = next
	s.ensureSelectionVisible()
	return oldSelected != s.selected || oldOffset != s.viewOffset
}

func (s *filesystemWidgetState) moveSelectionPage(direction int) bool {
	if len(s.entries) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	page := s.viewportRows
	if page < 1 {
		page = 1
	}

	oldSelected := s.selected
	oldOffset := s.viewOffset

	total := len(s.entries)
	maxOffset := total - page
	if maxOffset < 0 {
		maxOffset = 0
	}
	targetOffset := s.viewOffset + (direction * page)
	if targetOffset < 0 {
		targetOffset = 0
	}
	if targetOffset > maxOffset {
		targetOffset = maxOffset
	}

	deltaOffset := targetOffset - s.viewOffset
	s.viewOffset = targetOffset
	if deltaOffset != 0 {
		s.selected += deltaOffset
	} else {
		s.selected += direction * page
	}
	s.ensureSelectionVisible()
	return oldSelected != s.selected || oldOffset != s.viewOffset
}

func (s *filesystemWidgetState) moveSelectionTo(index int) bool {
	if len(s.entries) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	oldSelected := s.selected
	oldOffset := s.viewOffset
	if index < 0 {
		index = 0
	}
	if index >= len(s.entries) {
		index = len(s.entries) - 1
	}
	s.selected = index
	s.ensureSelectionVisible()
	return oldSelected != s.selected || oldOffset != s.viewOffset
}

func (s *filesystemWidgetState) requestBell() {
	s.bellPending = true
}

func (s *filesystemWidgetState) consumeBell() bool {
	if !s.bellPending {
		return false
	}
	s.bellPending = false
	return true
}

func (s *filesystemWidgetState) visibleRange() (int, int) {
	total := len(s.entries)
	if total == 0 {
		return 0, 0
	}
	if s.viewportRows <= 0 || s.viewportRows >= total {
		return 0, total
	}
	start := s.viewOffset
	if start < 0 {
		start = 0
	}
	maxStart := total - s.viewportRows
	if start > maxStart {
		start = maxStart
	}
	end := start + s.viewportRows
	if end > total {
		end = total
	}
	return start, end
}

func (s *filesystemWidgetState) ensureSelectionVisible() {
	total := len(s.entries)
	if total == 0 {
		s.selected = 0
		s.viewOffset = 0
		return
	}
	if s.selected < 0 {
		s.selected = 0
	}
	if s.selected >= total {
		s.selected = total - 1
	}
	if s.viewportRows <= 0 || s.viewportRows >= total {
		s.viewOffset = 0
		return
	}
	if s.viewOffset < 0 {
		s.viewOffset = 0
	}
	if s.selected < s.viewOffset {
		s.viewOffset = s.selected
	}
	lastVisible := s.viewOffset + s.viewportRows - 1
	if s.selected > lastVisible {
		s.viewOffset = s.selected - s.viewportRows + 1
	}
	maxOffset := total - s.viewportRows
	if s.viewOffset > maxOffset {
		s.viewOffset = maxOffset
	}
}

func (s *filesystemWidgetState) selectionKey(entry filesystemWidgetEntry) string {
	if strings.TrimSpace(entry.Path) != "" {
		return entry.Path
	}
	return entry.Name
}

func (s *filesystemWidgetState) isSoftSelected(entry filesystemWidgetEntry) bool {
	if len(s.softSelected) == 0 {
		return false
	}
	return s.softSelected[s.selectionKey(entry)]
}

func (s *filesystemWidgetState) toggleSoftSelection() error {
	entry, err := s.selectedEntry()
	if err != nil {
		return err
	}
	if s.softSelected == nil {
		s.softSelected = map[string]bool{}
	}
	key := s.selectionKey(entry)
	if s.softSelected[key] {
		delete(s.softSelected, key)
		return nil
	}
	s.softSelected[key] = true
	return nil
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

	parent := filepath.Dir(s.currentDir)
	if parent != s.currentDir {
		results = append([]filesystemWidgetEntry{{
			Name:   "..",
			Path:   parent,
			IsDir:  true,
			Size:   0,
			Exists: true,
		}}, results...)
	}
	s.entries = results
	if len(s.softSelected) > 0 {
		active := make(map[string]struct{}, len(s.entries))
		for _, entry := range s.entries {
			active[s.selectionKey(entry)] = struct{}{}
		}
		for key := range s.softSelected {
			if _, ok := active[key]; !ok {
				delete(s.softSelected, key)
			}
		}
	}
	if len(s.entries) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return nil
	}
	if s.selected >= len(s.entries) {
		s.selected = len(s.entries) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
	s.ensureSelectionVisible()
	return nil
}

func resolveFilesystemViewportRows() int {
	raw := strings.TrimSpace(os.Getenv("AGENTX_FILES_VIEWPORT_ROWS"))
	if raw == "" {
		return defaultFilesystemViewportRows
	}
	rows, err := strconv.Atoi(raw)
	if err != nil || rows <= 0 {
		return defaultFilesystemViewportRows
	}
	return rows
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
	if err := s.addEntryToContext(ctx, entry); err != nil {
		return err
	}
	s.status = "Added selected file to context"
	return nil
}

func (s *filesystemWidgetState) addSelectedToContext(ctx context.Context) (int, error) {
	entries, err := s.entriesForContextAction()
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return 0, errors.New("missing core HTTP base URL")
	}

	for _, entry := range entries {
		if err := s.addEntryToContext(ctx, entry); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

func (s *filesystemWidgetState) entriesForContextAction() ([]filesystemWidgetEntry, error) {
	softEntries := s.softSelectedEntriesInViewOrder()
	if len(softEntries) == 0 {
		entry, err := s.selectedEntry()
		if err != nil {
			return nil, err
		}
		if entry.IsDir {
			return nil, errors.New("selection is a directory; choose a file")
		}
		return []filesystemWidgetEntry{entry}, nil
	}

	fileEntries := make([]filesystemWidgetEntry, 0, len(softEntries))
	for _, entry := range softEntries {
		if !entry.IsDir {
			fileEntries = append(fileEntries, entry)
		}
	}
	if len(fileEntries) == 0 {
		return nil, errors.New("soft-selected entries are directories; choose a file")
	}
	return fileEntries, nil
}

func (s *filesystemWidgetState) addEntryToContext(ctx context.Context, entry filesystemWidgetEntry) error {
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

func (s *filesystemWidgetState) softSelectedEntriesInViewOrder() []filesystemWidgetEntry {
	if len(s.entries) == 0 || len(s.softSelected) == 0 {
		return nil
	}
	entries := make([]filesystemWidgetEntry, 0, len(s.softSelected))
	for _, entry := range s.entries {
		if s.isSoftSelected(entry) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (s *filesystemWidgetState) editSelected() (int, error) {
	entries, err := s.entriesForEditAction()
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if launchErr := launchEditorTmuxWindow(entry.Path); launchErr != nil {
			return 0, launchErr
		}
	}
	return len(entries), nil
}

func (s *filesystemWidgetState) entriesForEditAction() ([]filesystemWidgetEntry, error) {
	softEntries := s.softSelectedEntriesInViewOrder()
	if len(softEntries) == 0 {
		entry, err := s.selectedEntry()
		if err != nil {
			return nil, err
		}
		if entry.IsDir {
			return nil, errors.New("selection is a directory; choose a file")
		}
		return []filesystemWidgetEntry{entry}, nil
	}

	fileEntries := make([]filesystemWidgetEntry, 0, len(softEntries))
	for _, entry := range softEntries {
		if !entry.IsDir {
			fileEntries = append(fileEntries, entry)
		}
	}
	if len(fileEntries) == 0 {
		return nil, errors.New("soft-selected entries are directories; choose a file")
	}
	return fileEntries, nil
}

func launchEditorTmuxWindow(filePath string) error {
	sessionName, err := resolveTmuxSessionName()
	if err != nil {
		return err
	}

	editor := resolveEditor(os.Getenv("EDITOR"))
	windowName := buildEditorWindowName(filePath)
	editorCommand := buildEditorCommand(editor, filePath)
	projectDir := "."
	if envProjectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR")); envProjectDir != "" {
		projectDir = envProjectDir
	}

	driver, err := runtimeMultiplexerDriver(projectDir)
	if err != nil {
		return fmt.Errorf("failed to initialize multiplexer driver: %w", err)
	}
	if runErr := driver.Run(context.Background(), "new-window", "-t", sessionName+":", "-n", windowName, editorCommand); runErr != nil {
		return fmt.Errorf("failed launching editor window: %w", runErr)
	}
	return nil
}

func resolveTmuxSessionName() (string, error) {
	projectDir := "."
	if envProjectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR")); envProjectDir != "" {
		projectDir = envProjectDir
	}

	driver, err := runtimeMultiplexerDriver(projectDir)
	if err != nil {
		return "", fmt.Errorf("failed to initialize multiplexer driver: %w", err)
	}
	fromEnv := strings.TrimSpace(os.Getenv("AGENTX_TMUX_SESSION"))
	if fromEnv != "" {
		return fromEnv, nil
	}

	output, err := driver.Capture(context.Background(), "display-message", "-p", "#{session_name}")
	if err == nil {
		sessionName := strings.TrimSpace(output)
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
