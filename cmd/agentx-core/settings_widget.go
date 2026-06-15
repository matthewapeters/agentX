package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type settingsFieldKind string

const (
	settingsFieldEnum settingsFieldKind = "enum"
	settingsFieldBool settingsFieldKind = "bool"
	settingsFieldInt  settingsFieldKind = "int"
)

type settingsField struct {
	Section string
	Key     string
	Kind    settingsFieldKind
	Options []string
}

type settingsWidgetState struct {
	projectDir   string
	configPath   string
	fields       []settingsField
	values       map[string]string
	selected     int
	viewOffset   int
	viewportRows int
	viewportCols int
	showHelp     bool
	status       string
	bellPending  bool

	startupViewportApplied bool
}

const defaultSettingsViewportRows = 12
const defaultSettingsViewportCols = 58
const settingsWidgetBorderLines = 2

var approvedSettingsFields = []settingsField{
	{Section: "agentx", Key: "ollama_model", Kind: settingsFieldEnum, Options: []string{"qwen3.6:latest", "llama3.2", "phi4-mini:3.8b"}},
	{Section: "agentx", Key: "chat_backend", Kind: settingsFieldEnum, Options: []string{"ollama", "echo"}},
	{Section: "agentx", Key: "ollama_host", Kind: settingsFieldEnum, Options: []string{"localhost:11434", "127.0.0.1:11434"}},
	{Section: "agentx", Key: "chat_runtime", Kind: settingsFieldEnum, Options: []string{"go"}},
	{Section: "agentx", Key: "submit_timeout_seconds", Kind: settingsFieldInt, Options: []string{"30", "60", "120", "180"}},
	{Section: "agentx", Key: "submit_execution_timeout_seconds", Kind: settingsFieldInt, Options: []string{"30", "60", "120", "180"}},
	{Section: "agentx", Key: "context_history_session_sort", Kind: settingsFieldEnum, Options: []string{"Ascending", "Descending"}},
	{Section: "agentx", Key: "theme_mode", Kind: settingsFieldEnum, Options: []string{"Dark Mode", "Light Mode"}},
	{Section: "agentx", Key: "enable_gui_chat", Kind: settingsFieldBool, Options: []string{"true", "false"}},
	{Section: "tui", Key: "enable", Kind: settingsFieldBool, Options: []string{"true", "false"}},
	{Section: "tui", Key: "show_thinking", Kind: settingsFieldBool, Options: []string{"true", "false"}},
	{Section: "terminal", Key: "exec_mode", Kind: settingsFieldEnum, Options: []string{"autonomous", "confirm"}},
}

func runSettingsWidgetCommand(_ string, in io.Reader, out io.Writer) int {
	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	if projectDir == "" {
		projectDir = "."
	}
	absProjectDir, err := filepath.Abs(projectDir)
	if err == nil && strings.TrimSpace(absProjectDir) != "" {
		projectDir = absProjectDir
	}

	state := &settingsWidgetState{
		projectDir:   projectDir,
		configPath:   filepath.Join(projectDir, "agentx.toml"),
		fields:       append([]settingsField{}, approvedSettingsFields...),
		values:       map[string]string{},
		selected:     0,
		viewOffset:   0,
		viewportRows: defaultSettingsViewportRows,
		viewportCols: defaultSettingsViewportCols,
		status:       "Ready",
	}
	if err := state.reload(); err != nil {
		state.status = fmt.Sprintf("Load failed: %v", err)
	}

	if err := runSettingsWidgetLoop(context.Background(), in, out, state); err != nil {
		fmt.Fprintf(out, "Settings widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runSettingsWidgetLoop(ctx context.Context, in io.Reader, out io.Writer, state *settingsWidgetState) error {
	commandReader, promptMode, cleanup := newFilesystemWidgetCommandReader(in)
	defer cleanup()
	hideTerminalCursor(out)
	defer showTerminalCursor(out)
	startupHeight, startupWidth := resolveWidgetPaneSizeAtStartup(out)
	state.seedViewportFromStartup(startupHeight, startupWidth, promptMode)
	var previousLines []string

	for {
		state.adaptViewportToTerminal(out, promptMode)
		currentLines := filesystemWidgetFrameLines(state.render())
		if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
			return err
		}
		previousLines = currentLines
		if promptMode {
			if _, err := fmt.Fprint(out, "settings> "); err != nil {
				return err
			}
		}

		command, readErr := commandReader()
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
		command = normalizeSettingsWidgetControlCommand(command)

		action := handleWidgetLoopControlCommand(command, widgetLoopControlHandlers{
			QuitTokens:    []string{"q", "quit"},
			HelpTokens:    []string{"help"},
			RefreshTokens: []string{"r", "refresh"},
			OnHelp: func() {
				state.toggleHelp()
			},
			OnRefresh: func() {
				if err := state.reload(); err != nil {
					state.status = fmt.Sprintf("Error: %v", err)
					return
				}
				state.status = "Reloaded from agentx.toml"
			},
		})
		if action == widgetLoopControlQuit {
			return nil
		}
		if action == widgetLoopControlHandled {
			continue
		}

		if err := state.handleCommand(ctx, command); err != nil {
			state.status = fmt.Sprintf("Error: %v", err)
			continue
		}
		if state.consumeBell() {
			if _, err := fmt.Fprint(out, "\a"); err != nil {
				return err
			}
		}
	}
}

func normalizeSettingsWidgetControlCommand(raw string) string {
	return normalizeWidgetControlCommand(raw, defaultWidgetControlAliases())
}

func (s *settingsWidgetState) handleCommand(_ context.Context, command string) error {
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
		if !s.moveSelectionTo(len(s.fields) - 1) {
			s.status = "Bottom of list"
			s.requestBell()
		}
		return nil
	case "f", "right", "l", "enter", "space":
		return s.changeSelected(1)
	case "b", "left":
		return s.changeSelected(-1)
	case "r", "refresh":
		if err := s.reload(); err != nil {
			return err
		}
		s.status = "Reloaded from agentx.toml"
		return nil
	default:
		return fmt.Errorf("unsupported command: %s", command)
	}
}

func (s *settingsWidgetState) render() string {
	s.ensureSelectionVisible()
	start, end := s.visibleRange()
	total := len(s.fields)
	contentWidth := s.viewportContentWidth()
	horizontalBorder := strings.Repeat("─", contentWidth+2)

	lines := []string{
		"[SYSTEM SETTINGS]",
		trimSingleLine(fmt.Sprintf("config_file: %s", s.configPath), contentWidth+4),
		trimSingleLine("help: ? toggle | edit: Left/Right or Enter | reload: r | quit: q", contentWidth+4),
		trimSingleLine(fmt.Sprintf("showing %d-%d of %d", start+1, end, total), contentWidth+4),
		trimSingleLine(fmt.Sprintf("status: %s", strings.TrimSpace(s.status)), contentWidth+4),
		"",
	}
	if s.showHelp {
		lines = append(lines,
			trimSingleLine("approved fields only: model/backend/host/runtime/timeouts/theme/tui/terminal", contentWidth+4),
			trimSingleLine("changes write directly to agentx.toml and are constrained to known values", contentWidth+4),
		)
	}

	if total == 0 {
		lines = append(lines, "┌"+horizontalBorder+"┐")
		lines = append(lines, fmt.Sprintf("│ %-*s │", contentWidth, "(no approved settings configured)"))
		lines = append(lines, "└"+horizontalBorder+"┘")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "┌"+horizontalBorder+"┐")
	for idx := start; idx < end; idx++ {
		field := s.fields[idx]
		value := s.values[s.fieldKey(field)]
		row := s.formatRow(field, value, idx == s.selected)
		lines = append(lines, fmt.Sprintf("│ %-*s │", contentWidth, trimSingleLinePreserveSpacing(row, contentWidth)))
	}
	lines = append(lines, "└"+horizontalBorder+"┘")

	return strings.Join(lines, "\n")
}

func (s *settingsWidgetState) formatRow(field settingsField, value string, selected bool) string {
	marker := " "
	if selected {
		marker = ">"
	}
	contentWidth := s.viewportContentWidth()
	keyWidth := 34
	if keyWidth > contentWidth/2 {
		keyWidth = contentWidth / 2
	}
	if keyWidth < 16 {
		keyWidth = 16
	}
	valueWidth := contentWidth - 3 - keyWidth
	if valueWidth < 8 {
		valueWidth = 8
	}
	settingKey := field.Section + "." + field.Key
	return fmt.Sprintf("%s %-*s : %-*s", marker, keyWidth, trimSingleLinePreserveSpacing(settingKey, keyWidth), valueWidth, trimSingleLinePreserveSpacing(value, valueWidth))
}

func (s *settingsWidgetState) toggleHelp() {
	s.showHelp = !s.showHelp
	if s.showHelp {
		s.status = "Help shown"
		return
	}
	s.status = "Help hidden"
}

func (s *settingsWidgetState) headerLineCount() int {
	count := 6
	if s.showHelp {
		count += 2
	}
	return count
}

func (s *settingsWidgetState) adaptViewportToTerminal(out io.Writer, promptMode bool) {
	if s.startupViewportApplied {
		s.startupViewportApplied = false
		return
	}
	rows, cols := resolveWidgetViewport(out, s.headerLineCount(), settingsWidgetBorderLines, promptMode, defaultSettingsViewportCols, 20)
	s.viewportRows = rows
	s.viewportCols = cols
}

func (s *settingsWidgetState) seedViewportFromStartup(height int, width int, promptMode bool) {
	s.viewportRows = height - s.headerLineCount() - settingsWidgetBorderLines
	if promptMode {
		s.viewportRows--
	}
	if s.viewportRows < 1 {
		s.viewportRows = 1
	}
	s.viewportCols = defaultSettingsViewportCols
	if width > 4 {
		s.viewportCols = width - 4
	}
	if s.viewportCols < 20 {
		s.viewportCols = 20
	}
	s.startupViewportApplied = true
}

func (s *settingsWidgetState) viewportContentWidth() int {
	if s.viewportCols > 0 {
		return s.viewportCols
	}
	return defaultSettingsViewportCols
}

func (s *settingsWidgetState) moveSelection(delta int) bool {
	if len(s.fields) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	oldSel := s.selected
	oldOff := s.viewOffset
	next := s.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.fields) {
		next = len(s.fields) - 1
	}
	s.selected = next
	s.ensureSelectionVisible()
	return oldSel != s.selected || oldOff != s.viewOffset
}

func (s *settingsWidgetState) moveSelectionPage(direction int) bool {
	if len(s.fields) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	page := s.viewportRows
	if page < 1 {
		page = 1
	}
	oldSel := s.selected
	oldOff := s.viewOffset
	total := len(s.fields)
	maxOff := total - page
	if maxOff < 0 {
		maxOff = 0
	}
	target := s.viewOffset + (direction * page)
	if target < 0 {
		target = 0
	}
	if target > maxOff {
		target = maxOff
	}
	delta := target - s.viewOffset
	s.viewOffset = target
	if delta != 0 {
		s.selected += delta
	} else {
		s.selected += direction * page
	}
	s.ensureSelectionVisible()
	return oldSel != s.selected || oldOff != s.viewOffset
}

func (s *settingsWidgetState) moveSelectionTo(index int) bool {
	if len(s.fields) == 0 {
		s.selected = 0
		s.viewOffset = 0
		return false
	}
	oldSel := s.selected
	oldOff := s.viewOffset
	if index < 0 {
		index = 0
	}
	if index >= len(s.fields) {
		index = len(s.fields) - 1
	}
	s.selected = index
	s.ensureSelectionVisible()
	return oldSel != s.selected || oldOff != s.viewOffset
}

func (s *settingsWidgetState) ensureSelectionVisible() {
	total := len(s.fields)
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
	last := s.viewOffset + s.viewportRows - 1
	if s.selected > last {
		s.viewOffset = s.selected - s.viewportRows + 1
	}
	maxOff := total - s.viewportRows
	if s.viewOffset > maxOff {
		s.viewOffset = maxOff
	}
}

func (s *settingsWidgetState) visibleRange() (int, int) {
	total := len(s.fields)
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

func (s *settingsWidgetState) requestBell() { s.bellPending = true }

func (s *settingsWidgetState) consumeBell() bool {
	if !s.bellPending {
		return false
	}
	s.bellPending = false
	return true
}

func (s *settingsWidgetState) fieldKey(field settingsField) string {
	return field.Section + "." + field.Key
}

func (s *settingsWidgetState) reload() error {
	loaded := loadAgentXTomlSections(s.projectDir)
	flat := map[string]string{}
	for section, rows := range loaded {
		for _, row := range rows {
			flat[section+"."+row.Key] = row.Value
		}
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	for _, field := range s.fields {
		key := s.fieldKey(field)
		value := strings.TrimSpace(flat[key])
		if value == "" {
			if len(field.Options) > 0 {
				value = field.Options[0]
			}
		}
		s.values[key] = value
	}
	return nil
}

func (s *settingsWidgetState) changeSelected(direction int) error {
	if len(s.fields) == 0 {
		return nil
	}
	field := s.fields[s.selected]
	key := s.fieldKey(field)
	current := s.values[key]
	next, ok := cycleSettingValue(field, current, direction)
	if !ok {
		s.status = "No constrained options for this setting"
		return nil
	}
	if next == current {
		return nil
	}
	if err := updateAgentXTomlScalar(s.configPath, field.Section, field.Key, next, field.Kind); err != nil {
		return err
	}
	s.values[key] = next
	s.status = fmt.Sprintf("Saved %s=%s", key, trimSingleLine(next, 48))
	return nil
}

func cycleSettingValue(field settingsField, current string, direction int) (string, bool) {
	if len(field.Options) == 0 {
		return current, false
	}
	idx := 0
	for i, option := range field.Options {
		if option == current {
			idx = i
			break
		}
	}
	if direction >= 0 {
		idx = (idx + 1) % len(field.Options)
	} else {
		idx = (idx - 1 + len(field.Options)) % len(field.Options)
	}
	return field.Options[idx], true
}

func formatTomlValue(kind settingsFieldKind, value string) string {
	trimmed := strings.TrimSpace(value)
	switch kind {
	case settingsFieldBool:
		if strings.EqualFold(trimmed, "true") {
			return "true"
		}
		return "false"
	case settingsFieldInt:
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return trimmed
		}
		return "0"
	default:
		return strconv.Quote(trimmed)
	}
}

func updateAgentXTomlScalar(configPath string, section string, key string, value string, kind settingsFieldKind) error {
	formatted := fmt.Sprintf("%s = %s", key, formatTomlValue(kind, value))
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			content := fmt.Sprintf("[%s]\n%s\n", section, formatted)
			return os.WriteFile(configPath, []byte(content), 0o644)
		}
		return err
	}

	lines := strings.Split(string(raw), "\n")
	targetHeader := "[" + section + "]"
	sectionStart := -1
	sectionEnd := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == targetHeader {
			sectionStart = i
			continue
		}
		if sectionStart >= 0 && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionEnd = i
			break
		}
	}

	if sectionStart == -1 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, targetHeader, formatted)
		return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0o644)
	}

	for i := sectionStart + 1; i < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parsedKey, _, ok := parseTomlKeyValue(trimmed)
		if ok && parsedKey == key {
			lines[i] = formatted
			return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}

	inserted := append([]string{}, lines[:sectionEnd]...)
	inserted = append(inserted, formatted)
	inserted = append(inserted, lines[sectionEnd:]...)
	return os.WriteFile(configPath, []byte(strings.Join(inserted, "\n")), 0o644)
}
