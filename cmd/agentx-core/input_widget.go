package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type widgetActivitySnapshot struct {
	SessionID    string            `json:"session_id"`
	State        string            `json:"state"`
	Phase        string            `json:"phase"`
	PromptCycle  PromptCycleStatus `json:"prompt_cycle"`
	ContextFiles []string          `json:"context_files,omitempty"`
}

type widgetActivityState struct {
	mu          sync.RWMutex
	state       string
	phase       string
	doneUntil   time.Time
	failUntil   time.Time
	sessionID   string
	contextFile string
}

func newWidgetActivityState() *widgetActivityState {
	return &widgetActivityState{state: "idle", phase: "none"}
}

func (ws *widgetActivityState) update(snapshot widgetActivitySnapshot) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.state = strings.TrimSpace(snapshot.State)
	ws.phase = strings.TrimSpace(snapshot.Phase)
	ws.sessionID = strings.TrimSpace(snapshot.SessionID)
	now := time.Now()
	if ws.state == "completed" {
		ws.doneUntil = now.Add(1200 * time.Millisecond)
	}
	if ws.state == "failed" {
		ws.failUntil = now.Add(2000 * time.Millisecond)
	}
	ws.contextFile = ""
	if n := len(snapshot.ContextFiles); n > 0 {
		ws.contextFile = filepath.Base(strings.TrimSpace(snapshot.ContextFiles[n-1]))
	}
}

func (ws *widgetActivityState) promptLabel() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	now := time.Now()
	contextSuffix := ""
	if ws.contextFile != "" {
		contextSuffix = fmt.Sprintf("[ctx:%s]", ws.contextFile)
	}
	if ws.state == "working" {
		if ws.phase != "" && ws.phase != "none" {
			return fmt.Sprintf("agentx[%s]%s", ws.phase, contextSuffix)
		}
		return "agentx[working]" + contextSuffix
	}
	if now.Before(ws.failUntil) {
		return "agentx[failed]" + contextSuffix
	}
	if now.Before(ws.doneUntil) {
		return "agentx[done]" + contextSuffix
	}
	return "agentx" + contextSuffix
}

func startWidgetActivityPoller(baseURL string, target *widgetActivityState) func() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollCtx, pollCancel := context.WithTimeout(ctx, 1200*time.Millisecond)
				snapshot, err := fetchWidgetActivitySnapshot(pollCtx, baseURL)
				pollCancel()
				if err != nil {
					continue
				}
				target.update(snapshot)
			}
		}
	}()

	return func() {
		cancel()
		wg.Wait()
	}
}

func runInputWidgetCommand(coreHTTP string, in io.Reader, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Input widget failed: missing core HTTP base URL")
		return 1
	}
	baseURL = strings.TrimRight(baseURL, "/")
	activityState := newWidgetActivityState()
	stopPoller := startWidgetActivityPoller(baseURL, activityState)
	defer stopPoller()
	submitTimeout := resolveWidgetSubmitTimeout()

	submitPrompt := func(prompt string) (int, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), submitTimeout)
		response, err := submitPromptToCore(ctx, baseURL, prompt)
		cancel()
		if err != nil {
			fmt.Fprintf(out, "Submit failed: %v\n", err)
			if prompt == ":q" {
				return 1, true
			}
			return 0, false
		}

		switch prompt {
		case ":q":
			fmt.Fprintln(out, "Session shutdown requested.")
			_ = response
			return 0, true
		case ":clear":
			_ = response
		default:
			_ = response
		}

		return 0, false
	}

	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return runInputWidgetInteractiveLoop(file, out, activityState, submitPrompt)
	}

	return runInputWidgetLegacyLoop(in, out, activityState, submitPrompt)
}

func runInputWidgetLegacyLoop(in io.Reader, out io.Writer, activityState *widgetActivityState, submitPrompt func(string) (int, bool)) int {

	printInputWidgetLines(out,
		"Input ready. Enter prompt and press Enter.",
		"Commands: :q, :clear, :context-add <file-path> (alias :ctx-add), :multiline (alias :ml), :help",
		"Multiline mode: :multiline then enter lines, :submit/:send/:done to send, :cancel/:discard to discard",
	)

	scanner := bufio.NewScanner(in)
	multilineMode := false
	multilineLines := make([]string, 0, 16)

	for {
		promptLabel := formatInputPromptLabel(activityState.promptLabel(), out, multilineMode)
		if multilineMode {
			fmt.Fprintf(out, "%s[multi]> ", promptLabel)
		} else {
			fmt.Fprintf(out, "%s> ", promptLabel)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(out, "Input widget failed: %v\n", err)
				return 1
			}
			return 0
		}

		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		if multilineMode {
			switch strings.ToLower(trimmedLine) {
			case ":help", ":commands":
				printInputWidgetLines(out,
					"Multiline controls:",
					"  :submit | :send | :done       Send the multiline prompt",
					"  :cancel | :discard            Discard multiline input",
					"  :help                         Show multiline controls",
				)
				continue
			case ":submit", ":send", ":done":
				prompt := strings.Join(multilineLines, "\n")
				if strings.TrimSpace(prompt) == "" {
					printInputWidgetLines(out, "Multiline buffer is empty; add text or use :cancel.")
					continue
				}

				multilineMode = false
				multilineLines = multilineLines[:0]
				// Remove the just-submitted input line so acceptance feedback appears immediately.
				fmt.Fprint(out, "\033[1A\033[2K\r")

				exitCode, shouldExit := submitPrompt(prompt)
				if shouldExit {
					return exitCode
				}
				continue
			case ":cancel", ":discard":
				multilineMode = false
				multilineLines = multilineLines[:0]
				printInputWidgetLines(out, "Multiline input cancelled.")
				continue
			default:
				multilineLines = append(multilineLines, line)
				continue
			}
		}

		prompt := trimmedLine
		if prompt == "" {
			continue
		}

		switch strings.ToLower(prompt) {
		case ":help", ":commands":
			printInputWidgetLines(out,
				"Available commands:",
				"  :q                     Shut down the current session",
				"  :clear                 Clear input panel only",
				"  :context-add <path>    Register a context file",
				"  :ctx-add <path>        Alias for :context-add",
				"  :multiline             Enter multiline compose mode",
				"  :ml                    Alias for :multiline",
				"Multiline controls once active: :submit/:send/:done, :cancel/:discard",
			)
			continue
		case ":multiline", ":ml":
			multilineMode = true
			multilineLines = multilineLines[:0]
			printInputWidgetLines(out,
				"Multiline mode enabled. Enter message lines.",
				"Use :submit to send or :cancel to discard.",
			)
			continue
		}

		// Remove the just-submitted input line so acceptance feedback appears immediately.
		fmt.Fprint(out, "\033[1A\033[2K\r")
		exitCode, shouldExit := submitPrompt(prompt)
		if shouldExit {
			return exitCode
		}
	}
}

type inputWidgetFocus int

const (
	inputWidgetFocusInput inputWidgetFocus = iota
	inputWidgetFocusControl
)

type inputWidgetKey struct {
	kind string
	raw  string
}

type inputWidgetAction struct {
	submitPrompt string
	quitOnSubmit bool
}

type inputWidgetComposeState struct {
	focus inputWidgetFocus

	showHelp bool
	status   string

	inputLines [][]rune
	cursorRow  int
	cursorCol  int
	viewRow    int
	viewCol    int

	control       []rune
	controlCursor int

	viewportRows int
	viewportCols int
}

func newInputWidgetComposeState() *inputWidgetComposeState {
	return &inputWidgetComposeState{
		focus:        inputWidgetFocusInput,
		status:       "Ready",
		inputLines:   [][]rune{[]rune{}},
		viewportRows: 8,
		viewportCols: 56,
	}
}

func runInputWidgetInteractiveLoop(in *os.File, out io.Writer, activityState *widgetActivityState, submitPrompt func(string) (int, bool)) int {
	state := newInputWidgetComposeState()
	state.status = "ESC toggles focus; :? toggles help; :q exits"

	commandReader, cleanup, err := newInputWidgetKeyReader(in)
	if err != nil {
		fmt.Fprintf(out, "Input widget failed: %v\n", err)
		return 1
	}
	defer cleanup()

	var previousLines []string

	for {
		state.adaptViewportToTerminal(out)
		currentLines := filesystemWidgetFrameLines(state.render(activityState.promptLabel()))
		if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
			fmt.Fprintf(out, "Input widget failed: %v\n", err)
			return 1
		}
		previousLines = currentLines

		key, err := commandReader()
		if err != nil {
			if err == io.EOF {
				return 0
			}
			fmt.Fprintf(out, "Input widget failed: %v\n", err)
			return 1
		}

		action := state.handleKey(key)
		if strings.TrimSpace(action.submitPrompt) != "" {
			exitCode, shouldExit := submitPrompt(action.submitPrompt)
			if shouldExit {
				return exitCode
			}
			if action.quitOnSubmit {
				return 0
			}
		}
	}
}

func newInputWidgetKeyReader(file *os.File) (func() (inputWidgetKey, error), func(), error) {
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return nil, nil, fmt.Errorf("interactive reader requires terminal input")
	}

	originalState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, nil, err
	}

	type byteResult struct {
		b   byte
		err error
	}
	byteChan := make(chan byteResult, 256)
	stop := make(chan struct{})

	go func() {
		buf := make([]byte, 1)
		for {
			n, readErr := file.Read(buf)
			if n > 0 {
				select {
				case byteChan <- byteResult{buf[0], nil}:
				case <-stop:
					return
				}
			}
			if readErr != nil {
				select {
				case byteChan <- byteResult{0, readErr}:
				case <-stop:
				}
				return
			}
		}
	}()

	readByteTimeout := func(timeout time.Duration) (byte, bool, error) {
		if timeout <= 0 {
			select {
			case r := <-byteChan:
				return r.b, true, r.err
			case <-stop:
				return 0, false, io.EOF
			}
		}
		select {
		case r := <-byteChan:
			return r.b, true, r.err
		case <-time.After(timeout):
			return 0, false, nil
		case <-stop:
			return 0, false, io.EOF
		}
	}

	readCommand := func() (inputWidgetKey, error) {
		for {
			b, ok, readErr := readByteTimeout(0)
			if !ok {
				return inputWidgetKey{}, io.EOF
			}
			if readErr != nil {
				return inputWidgetKey{}, readErr
			}
			switch b {
			case 3:
				widgetKeyDebug("\\x03", "ctrl_c")
				return inputWidgetKey{kind: "ctrl_c"}, nil
			case 13, 10:
				widgetKeyDebug("\\r/\\n", "enter")
				return inputWidgetKey{kind: "enter"}, nil
			case 9:
				widgetKeyDebug("\\t", "tab")
				return inputWidgetKey{kind: "tab"}, nil
			case 127:
				widgetKeyDebug("\\x7f", "backspace")
				return inputWidgetKey{kind: "backspace"}, nil
			case 27:
				key, ok2, escErr := readInputWidgetEscapeKey(readByteTimeout)
				if escErr != nil {
					return inputWidgetKey{}, escErr
				}
				if ok2 {
					return key, nil
				}
				widgetKeyDebug("\\x1b", "esc")
				return inputWidgetKey{kind: "esc"}, nil
			default:
				if b < 32 {
					continue
				}
				raw := string([]byte{b})
				widgetKeyDebug(raw, "text")
				return inputWidgetKey{kind: "text", raw: raw}, nil
			}
		}
	}

	cleanup := func() {
		close(stop)
		_ = term.Restore(fd, originalState)
	}

	return readCommand, cleanup, nil
}

func readInputWidgetEscapeKey(readByte func(time.Duration) (byte, bool, error)) (inputWidgetKey, bool, error) {
	next, ok, err := readByte(50 * time.Millisecond)
	if err != nil {
		return inputWidgetKey{}, false, err
	}
	if !ok || next != '[' {
		return inputWidgetKey{}, false, nil
	}

	seq := make([]byte, 0, 8)
	for {
		b, ok2, readErr := readByte(50 * time.Millisecond)
		if readErr != nil {
			return inputWidgetKey{}, false, readErr
		}
		if !ok2 {
			return inputWidgetKey{}, false, nil
		}
		seq = append(seq, b)
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
			break
		}
		if len(seq) > 8 {
			return inputWidgetKey{}, false, nil
		}
	}

	raw := "\x1b[" + string(seq)
	kind, ok3 := normalizeInputWidgetEscapeSequence(raw)
	if !ok3 {
		widgetKeyDebug(raw, "(none)")
		return inputWidgetKey{}, false, nil
	}
	widgetKeyDebug(raw, kind)
	return inputWidgetKey{kind: kind}, true, nil
}

func normalizeInputWidgetEscapeSequence(raw string) (string, bool) {
	switch raw {
	case "\x1b[A":
		return "up", true
	case "\x1b[B":
		return "down", true
	case "\x1b[C":
		return "right", true
	case "\x1b[D":
		return "left", true
	case "\x1b[1;2A", "\x1b[a":
		return "shift_up", true
	case "\x1b[1;2B", "\x1b[b":
		return "shift_down", true
	case "\x1b[1;2C", "\x1b[c":
		return "shift_right", true
	case "\x1b[1;2D", "\x1b[d":
		return "shift_left", true
	default:
		return "", false
	}
}

func (s *inputWidgetComposeState) handleKey(key inputWidgetKey) inputWidgetAction {
	s.ensureInputInitialized()
	if key.kind == "ctrl_c" {
		return inputWidgetAction{submitPrompt: ":q", quitOnSubmit: true}
	}

	if key.kind == "esc" {
		s.toggleFocus()
		return inputWidgetAction{}
	}

	if s.focus == inputWidgetFocusControl {
		return s.handleControlKey(key)
	}
	return s.handleInputKey(key)
}

func (s *inputWidgetComposeState) handleControlKey(key inputWidgetKey) inputWidgetAction {
	switch key.kind {
	case "left":
		if s.controlCursor > 0 {
			s.controlCursor--
		}
	case "right":
		if s.controlCursor < len(s.control) {
			s.controlCursor++
		}
	case "backspace":
		if s.controlCursor > 0 {
			copy(s.control[s.controlCursor-1:], s.control[s.controlCursor:])
			s.control = s.control[:len(s.control)-1]
			s.controlCursor--
		}
	case "text":
		if key.raw != "" {
			r := []rune(key.raw)
			s.insertControlRunes(r)
		}
	case "enter":
		return s.applyControlCommand()
	}
	return inputWidgetAction{}
}

func (s *inputWidgetComposeState) handleInputKey(key inputWidgetKey) inputWidgetAction {
	switch key.kind {
	case "left":
		s.moveCursorLeft()
	case "right":
		s.moveCursorRight()
	case "up":
		s.moveCursorUp()
	case "down":
		s.moveCursorDown()
	case "shift_left":
		s.panView(-1, 0)
	case "shift_right":
		s.panView(1, 0)
	case "shift_up":
		s.panView(0, -1)
	case "shift_down":
		s.panView(0, 1)
	case "backspace":
		s.backspaceInput()
	case "tab":
		s.insertInputText("\t")
	case "enter":
		s.insertNewline()
	case "text":
		s.insertInputText(key.raw)
	}
	return inputWidgetAction{}
}

func (s *inputWidgetComposeState) applyControlCommand() inputWidgetAction {
	raw := strings.TrimSpace(string(s.control))
	if raw == "" || raw == ":" {
		prompt := s.inputText()
		if strings.TrimSpace(prompt) == "" {
			s.status = "Input is empty"
			s.focus = inputWidgetFocusInput
			return inputWidgetAction{}
		}
		s.clearInput()
		s.clearControl()
		s.focus = inputWidgetFocusInput
		s.status = "Submitted input"
		return inputWidgetAction{submitPrompt: prompt}
	}

	if raw == ":?" {
		s.showHelp = !s.showHelp
		if s.showHelp {
			s.status = "Help shown"
		} else {
			s.status = "Help hidden"
		}
		s.clearControl()
		s.focus = inputWidgetFocusInput
		return inputWidgetAction{}
	}

	if raw == ":q" {
		s.status = "Session shutdown requested"
		s.clearControl()
		return inputWidgetAction{submitPrompt: ":q", quitOnSubmit: true}
	}

	s.status = "Submitted control command"
	s.clearControl()
	s.focus = inputWidgetFocusInput
	return inputWidgetAction{submitPrompt: raw}
}

func (s *inputWidgetComposeState) toggleFocus() {
	if s.focus == inputWidgetFocusInput {
		s.focus = inputWidgetFocusControl
		if len(s.control) == 0 {
			s.control = []rune{':'}
			s.controlCursor = len(s.control)
		}
		s.status = "Control entry focus"
		return
	}
	s.focus = inputWidgetFocusInput
	s.status = "Input focus"
}

func (s *inputWidgetComposeState) clearControl() {
	s.control = s.control[:0]
	s.controlCursor = 0
}

func (s *inputWidgetComposeState) clearInput() {
	s.inputLines = [][]rune{[]rune{}}
	s.cursorRow = 0
	s.cursorCol = 0
	s.viewRow = 0
	s.viewCol = 0
}

func (s *inputWidgetComposeState) insertControlRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	head := append([]rune{}, s.control[:s.controlCursor]...)
	tail := append([]rune{}, s.control[s.controlCursor:]...)
	head = append(head, runes...)
	s.control = append(head, tail...)
	s.controlCursor += len(runes)
}

func (s *inputWidgetComposeState) ensureInputInitialized() {
	if len(s.inputLines) == 0 {
		s.inputLines = [][]rune{[]rune{}}
	}
	if s.cursorRow < 0 {
		s.cursorRow = 0
	}
	if s.cursorRow >= len(s.inputLines) {
		s.cursorRow = len(s.inputLines) - 1
	}
	if s.cursorCol < 0 {
		s.cursorCol = 0
	}
	if s.cursorCol > len(s.inputLines[s.cursorRow]) {
		s.cursorCol = len(s.inputLines[s.cursorRow])
	}
}

func (s *inputWidgetComposeState) insertInputText(text string) {
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	s.ensureInputInitialized()
	line := s.inputLines[s.cursorRow]
	head := append([]rune{}, line[:s.cursorCol]...)
	tail := append([]rune{}, line[s.cursorCol:]...)
	head = append(head, runes...)
	head = append(head, tail...)
	s.inputLines[s.cursorRow] = head
	s.cursorCol += len(runes)
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) insertNewline() {
	s.ensureInputInitialized()
	line := s.inputLines[s.cursorRow]
	left := append([]rune{}, line[:s.cursorCol]...)
	right := append([]rune{}, line[s.cursorCol:]...)
	s.inputLines[s.cursorRow] = left
	index := s.cursorRow + 1
	tail := append([][]rune{right}, s.inputLines[index:]...)
	s.inputLines = append(s.inputLines[:index], tail...)
	s.cursorRow++
	s.cursorCol = 0
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) backspaceInput() {
	s.ensureInputInitialized()
	if s.cursorCol > 0 {
		line := s.inputLines[s.cursorRow]
		copy(line[s.cursorCol-1:], line[s.cursorCol:])
		s.inputLines[s.cursorRow] = line[:len(line)-1]
		s.cursorCol--
		s.ensureCursorVisible()
		return
	}
	if s.cursorRow == 0 {
		return
	}
	prev := s.inputLines[s.cursorRow-1]
	line := s.inputLines[s.cursorRow]
	newCol := len(prev)
	merged := append(prev, line...)
	s.inputLines[s.cursorRow-1] = merged
	copy(s.inputLines[s.cursorRow:], s.inputLines[s.cursorRow+1:])
	s.inputLines = s.inputLines[:len(s.inputLines)-1]
	s.cursorRow--
	s.cursorCol = newCol
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) moveCursorLeft() {
	s.ensureInputInitialized()
	if s.cursorCol > 0 {
		s.cursorCol--
	} else if s.cursorRow > 0 {
		s.cursorRow--
		s.cursorCol = len(s.inputLines[s.cursorRow])
	}
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) moveCursorRight() {
	s.ensureInputInitialized()
	lineLen := len(s.inputLines[s.cursorRow])
	if s.cursorCol < lineLen {
		s.cursorCol++
	} else if s.cursorRow+1 < len(s.inputLines) {
		s.cursorRow++
		s.cursorCol = 0
	}
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) moveCursorUp() {
	s.ensureInputInitialized()
	if s.cursorRow > 0 {
		s.cursorRow--
		if s.cursorCol > len(s.inputLines[s.cursorRow]) {
			s.cursorCol = len(s.inputLines[s.cursorRow])
		}
	}
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) moveCursorDown() {
	s.ensureInputInitialized()
	if s.cursorRow+1 < len(s.inputLines) {
		s.cursorRow++
		if s.cursorCol > len(s.inputLines[s.cursorRow]) {
			s.cursorCol = len(s.inputLines[s.cursorRow])
		}
	}
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) panView(deltaCol int, deltaRow int) {
	maxRowOffset, maxColOffset := s.maxViewportOffsets()
	s.viewCol += deltaCol
	s.viewRow += deltaRow
	if s.viewCol < 0 {
		s.viewCol = 0
	}
	if s.viewRow < 0 {
		s.viewRow = 0
	}
	if s.viewCol > maxColOffset {
		s.viewCol = maxColOffset
	}
	if s.viewRow > maxRowOffset {
		s.viewRow = maxRowOffset
	}
}

func (s *inputWidgetComposeState) maxViewportOffsets() (int, int) {
	maxRow := len(s.inputLines) - s.viewportRows
	if maxRow < 0 {
		maxRow = 0
	}
	maxLine := 0
	for _, line := range s.inputLines {
		if len(line) > maxLine {
			maxLine = len(line)
		}
	}
	maxCol := maxLine - s.viewportCols
	if maxCol < 0 {
		maxCol = 0
	}
	return maxRow, maxCol
}

func (s *inputWidgetComposeState) ensureCursorVisible() {
	if s.cursorRow < s.viewRow {
		s.viewRow = s.cursorRow
	}
	if s.cursorRow >= s.viewRow+s.viewportRows {
		s.viewRow = s.cursorRow - s.viewportRows + 1
	}
	if s.cursorCol < s.viewCol {
		s.viewCol = s.cursorCol
	}
	if s.cursorCol >= s.viewCol+s.viewportCols {
		s.viewCol = s.cursorCol - s.viewportCols + 1
	}
	if s.viewRow < 0 {
		s.viewRow = 0
	}
	if s.viewCol < 0 {
		s.viewCol = 0
	}
	maxRowOffset, maxColOffset := s.maxViewportOffsets()
	if s.viewRow > maxRowOffset {
		s.viewRow = maxRowOffset
	}
	if s.viewCol > maxColOffset {
		s.viewCol = maxColOffset
	}
}

func (s *inputWidgetComposeState) inputText() string {
	parts := make([]string, 0, len(s.inputLines))
	for _, line := range s.inputLines {
		parts = append(parts, string(line))
	}
	return strings.Join(parts, "\n")
}

func (s *inputWidgetComposeState) adaptViewportToTerminal(out io.Writer) {
	height, width := resolveWidgetPaneSizeForWriter(out)
	if width < 44 {
		width = 44
	}
	header := 4
	if s.showHelp {
		header += 2
	}
	controlOuter := 3
	inputOuter := height - header - controlOuter
	if inputOuter < 5 {
		inputOuter = 5
	}
	inputInner := inputOuter - 2
	textRows := inputInner - 1
	if textRows < 1 {
		textRows = 1
	}
	textCols := width - 6
	if textCols < 12 {
		textCols = 12
	}
	s.viewportRows = textRows
	s.viewportCols = textCols
	s.ensureCursorVisible()
}

func (s *inputWidgetComposeState) render(activityLabel string) string {
	s.ensureInputInitialized()
	s.ensureCursorVisible()
	inputColor := ansiBlue
	controlColor := ansiBlue
	if s.focus == inputWidgetFocusInput {
		inputColor = ansiCyan
	} else {
		controlColor = ansiCyan
	}

	lines := []string{
		"[INPUT]",
		trimSingleLine(fmt.Sprintf("activity: %s", activityLabel), 96),
	}
	if s.showHelp {
		lines = append(lines,
			trimSingleLine("help: arrows move cursor | Shift+arrows pan view | Tab inserts tab", 96),
			trimSingleLine("control: ESC then :q quit | :? toggle help | Enter submit from control", 96),
		)
	}
	lines = append(lines, "")
	lines = append(lines, s.renderInputBox(inputColor)...)
	lines = append(lines, "")
	lines = append(lines, s.renderControlBox(controlColor)...)
	return strings.Join(lines, "\n")
}

func (s *inputWidgetComposeState) focusLabel() string {
	if s.focus == inputWidgetFocusInput {
		return "input"
	}
	return "control"
}

func (s *inputWidgetComposeState) renderInputBox(color string) []string {
	textRows := s.viewportRows
	textCols := s.viewportCols
	vOverflow := len(s.inputLines) > textRows
	maxLine := 0
	for _, line := range s.inputLines {
		if len(line) > maxLine {
			maxLine = len(line)
		}
	}
	hOverflow := maxLine > textCols
	trackRowStart, trackRowLen := scrollbarThumb(len(s.inputLines), textRows, s.viewRow, textRows)
	trackColStart, trackColLen := scrollbarThumb(maxLine, textCols, s.viewCol, textCols)

	top := color + "┌" + strings.Repeat("─", textCols+3) + "┐" + ansiReset
	rows := []string{top}
	for i := 0; i < textRows; i++ {
		lineIndex := s.viewRow + i
		content := s.renderInputViewportRow(lineIndex, textCols)
		scrollCell := " "
		if vOverflow {
			if i >= trackRowStart && i < trackRowStart+trackRowLen {
				scrollCell = ansiReverse + "█" + ansiReset
			} else {
				scrollCell = ansiReverse + " " + ansiReset
			}
		}
		rows = append(rows, color+"│ "+content+scrollCell+" │"+ansiReset)
	}
	rows = append(rows, color+"│ "+s.renderHorizontalScrollbar(textCols, hOverflow, trackColStart, trackColLen)+" │"+ansiReset)
	rows = append(rows, color+"└"+strings.Repeat("─", textCols+3)+"┘"+ansiReset)
	return rows
}

func (s *inputWidgetComposeState) renderInputViewportRow(lineIndex int, cols int) string {
	line := []rune{}
	if lineIndex >= 0 && lineIndex < len(s.inputLines) {
		line = s.inputLines[lineIndex]
	}
	start := s.viewCol
	if start < 0 {
		start = 0
	}
	if start > len(line) {
		start = len(line)
	}
	end := start + cols
	if end > len(line) {
		end = len(line)
	}
	visible := make([]string, 0, cols)
	for _, r := range line[start:end] {
		visible = append(visible, string(r))
	}
	for len(visible) < cols {
		visible = append(visible, " ")
	}

	if s.focus == inputWidgetFocusInput && lineIndex == s.cursorRow {
		cursor := s.cursorCol - s.viewCol
		if cursor >= 0 && cursor < cols {
			ch := visible[cursor]
			if ch == "" || ch == " " {
				ch = " "
			}
			visible[cursor] = ansiReverse + ch + ansiReset
		}
	}
	return strings.Join(visible, "")
}

func (s *inputWidgetComposeState) renderHorizontalScrollbar(trackLen int, overflow bool, thumbStart int, thumbLen int) string {
	if trackLen < 1 {
		return " "
	}
	if !overflow {
		return strings.Repeat(" ", trackLen+1)
	}
	parts := make([]string, 0, trackLen+1)
	for i := 0; i < trackLen; i++ {
		if i >= thumbStart && i < thumbStart+thumbLen {
			parts = append(parts, ansiReverse+"█"+ansiReset)
		} else {
			parts = append(parts, ansiReverse+" "+ansiReset)
		}
	}
	parts = append(parts, ansiReverse+" "+ansiReset)
	return strings.Join(parts, "")
}

func (s *inputWidgetComposeState) renderControlBox(color string) []string {
	cols := s.viewportCols + 1
	if cols < 16 {
		cols = 16
	}
	content := s.renderControlContent(cols)
	return []string{
		color + "┌" + strings.Repeat("─", cols+2) + "┐" + ansiReset,
		color + "│ " + content + " │" + ansiReset,
		color + "└" + strings.Repeat("─", cols+2) + "┘" + ansiReset,
	}
}

func (s *inputWidgetComposeState) renderControlContent(cols int) string {
	if cols < 1 {
		cols = 1
	}
	prefix := []rune(":")
	text := append([]rune{}, s.control...)
	if len(text) == 0 {
		text = []rune{}
	}
	full := append(prefix, text...)
	cursor := 1 + s.controlCursor
	start := 0
	if cursor >= cols {
		start = cursor - cols + 1
	}
	if start < 0 {
		start = 0
	}
	if start > len(full) {
		start = len(full)
	}
	end := start + cols
	if end > len(full) {
		end = len(full)
	}
	visible := make([]string, 0, cols)
	for _, r := range full[start:end] {
		visible = append(visible, string(r))
	}
	for len(visible) < cols {
		visible = append(visible, " ")
	}
	if s.focus == inputWidgetFocusControl {
		cursorPos := cursor - start
		if cursorPos >= 0 && cursorPos < cols {
			ch := visible[cursorPos]
			if ch == "" || ch == " " {
				ch = " "
			}
			visible[cursorPos] = ansiReverse + ch + ansiReset
		}
	}
	return strings.Join(visible, "")
}

func scrollbarThumb(total int, visible int, offset int, track int) (int, int) {
	if total <= 0 || visible <= 0 || track <= 0 || total <= visible {
		return 0, 0
	}
	thumb := (visible*track + total - 1) / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > track {
		thumb = track
	}
	maxOffset := total - visible
	if maxOffset < 1 {
		return 0, thumb
	}
	maxTrackStart := track - thumb
	if maxTrackStart < 0 {
		maxTrackStart = 0
	}
	start := 0
	if maxTrackStart > 0 {
		start = (offset*maxTrackStart + maxOffset/2) / maxOffset
	}
	if start < 0 {
		start = 0
	}
	if start > maxTrackStart {
		start = maxTrackStart
	}
	return start, thumb
}

func inputWidgetPaneWidth(out io.Writer) int {
	_, width := resolveWidgetPaneSizeForWriter(out)
	if width < 20 {
		return 20
	}
	return width
}

func printInputWidgetLines(out io.Writer, lines ...string) {
	width := inputWidgetPaneWidth(out)
	lineWidth := width - 1
	if lineWidth < 12 {
		lineWidth = 12
	}
	for _, raw := range lines {
		wrapped := wrapTextLines(raw, lineWidth)
		for _, line := range wrapped {
			fmt.Fprintln(out, line)
		}
	}
}

func formatInputPromptLabel(label string, out io.Writer, multiline bool) string {
	width := inputWidgetPaneWidth(out)
	reserved := 2 // "> "
	if multiline {
		reserved = len("[multi]> ")
	}
	maxLen := width - reserved
	if maxLen < 8 {
		maxLen = 8
	}
	return trimSingleLine(label, maxLen)
}

func resolveWidgetSubmitTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AGENTX_SUBMIT_TIMEOUT_SEC"))
	if raw == "" {
		return 120 * time.Second
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 120 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

type submitErrorResponse struct {
	Error string `json:"error"`
}

func submitPromptToCore(ctx context.Context, baseURL string, prompt string) (string, error) {
	payload := submitRequest{Prompt: prompt}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/submit", bytes.NewReader(encodedPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorPayload submitErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errorPayload); decodeErr == nil && strings.TrimSpace(errorPayload.Error) != "" {
			return "", fmt.Errorf(errorPayload.Error)
		}
		return "", fmt.Errorf("submit failed with status %d", resp.StatusCode)
	}

	var success submitResponse
	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return "", err
	}
	if strings.TrimSpace(success.Response) == "" {
		return "", fmt.Errorf("submit endpoint returned empty response")
	}

	return success.Response, nil
}

func fetchWidgetActivitySnapshot(ctx context.Context, baseURL string) (widgetActivitySnapshot, error) {
	var snapshot widgetActivitySnapshot
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/activity", nil)
	if err != nil {
		return snapshot, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return snapshot, fmt.Errorf("activity failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return snapshot, err
	}
	if strings.TrimSpace(snapshot.State) == "" {
		snapshot.State = "idle"
	}
	if strings.TrimSpace(snapshot.Phase) == "" {
		snapshot.Phase = "none"
	}
	return snapshot, nil
}
