package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

var (
	widgetKeyDebugOnce    sync.Once
	widgetKeyDebugEnabled bool
)

var widgetDefaultControlAliases = map[string]string{
	"?":        "help",
	"controls": "help",
	"exit":     "quit",
}

func defaultWidgetControlAliases() map[string]string {
	return widgetDefaultControlAliases
}

func isWidgetKeyDebugEnabled() bool {
	widgetKeyDebugOnce.Do(func() {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv("AGENTX_WIDGET_KEY_DEBUG")))
		switch raw {
		case "1", "true", "yes", "on", "debug":
			widgetKeyDebugEnabled = true
		default:
			widgetKeyDebugEnabled = false
		}
	})
	return widgetKeyDebugEnabled
}

func widgetKeyDebug(raw string, normalized string) {
	if !isWidgetKeyDebugEnabled() {
		return
	}
	if strings.TrimSpace(normalized) == "" {
		normalized = "(none)"
	}
	_, _ = fmt.Fprintf(os.Stderr, "[widget-key] raw=%s normalized=%q\n", strconv.QuoteToASCII(raw), normalized)
}

func newWidgetCommandReader(in io.Reader, normalize func(string) string) (func() (string, error), bool, func()) {
	if normalize == nil {
		normalize = func(raw string) string {
			return strings.ToLower(strings.TrimSpace(raw))
		}
	}

	file, ok := in.(*os.File)
	if !ok {
		scanner := bufio.NewScanner(in)
		return func() (string, error) {
			if !scanner.Scan() {
				if scanErr := scanner.Err(); scanErr != nil {
					return "", scanErr
				}
				return "", io.EOF
			}
			raw := scanner.Text()
			normalized := normalize(raw)
			widgetKeyDebug(raw, normalized)
			return normalized, nil
		}, true, func() {}
	}

	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		scanner := bufio.NewScanner(file)
		return func() (string, error) {
			if !scanner.Scan() {
				if scanErr := scanner.Err(); scanErr != nil {
					return "", scanErr
				}
				return "", io.EOF
			}
			raw := scanner.Text()
			normalized := normalize(raw)
			widgetKeyDebug(raw, normalized)
			return normalized, nil
		}, true, func() {}
	}

	originalState, err := term.MakeRaw(fd)
	if err != nil {
		scanner := bufio.NewScanner(file)
		return func() (string, error) {
			if !scanner.Scan() {
				if scanErr := scanner.Err(); scanErr != nil {
					return "", scanErr
				}
				return "", io.EOF
			}
			raw := scanner.Text()
			normalized := normalize(raw)
			widgetKeyDebug(raw, normalized)
			return normalized, nil
		}, true, func() {}
	}

	reader := bufio.NewReader(file)
	readCommand := func() (string, error) {
		for {
			b, readErr := reader.ReadByte()
			if readErr != nil {
				return "", readErr
			}
			switch b {
			case 3:
				normalized := normalize("ctrl_c")
				widgetKeyDebug("\\x03", normalized)
				return normalized, nil
			case 13, 10:
				widgetKeyDebug("\\r/\\n", "enter")
				return "enter", nil
			case 9:
				widgetKeyDebug("\\t", "tab")
				return "tab", nil
			case ' ':
				widgetKeyDebug(" ", "space")
				return "space", nil
			case 27:
				cmd, ok, err := readWidgetEscapeCommand(reader)
				if err != nil {
					return "", err
				}
				if ok {
					return cmd, nil
				}
				widgetKeyDebug("\\x1b", "(none)")
			case 127:
				normalized := normalize("backspace")
				widgetKeyDebug("\\x7f", normalized)
				return normalized, nil
			default:
				if b < 32 {
					continue
				}
				raw := string([]byte{b})
				normalized := normalize(raw)
				widgetKeyDebug(raw, normalized)
				return normalized, nil
			}
		}
	}

	cleanup := func() {
		_ = term.Restore(fd, originalState)
	}

	return readCommand, false, cleanup
}

func readWidgetEscapeCommand(reader *bufio.Reader) (string, bool, error) {
	next, err := reader.ReadByte()
	if err != nil {
		return "", false, err
	}
	if next != '[' {
		return "", false, nil
	}

	seq := make([]byte, 0, 8)
	for {
		b, readErr := reader.ReadByte()
		if readErr != nil {
			return "", false, readErr
		}
		seq = append(seq, b)
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
			break
		}
		if len(seq) > 8 {
			return "", false, nil
		}
	}

	raw := "\x1b[" + string(seq)
	command, ok := normalizeWidgetEscapeSequence(raw)
	if !ok {
		widgetKeyDebug(raw, "(none)")
		return "", false, nil
	}
	widgetKeyDebug(raw, command)
	return command, true, nil
}

func normalizeWidgetCommand(raw string) string {
	if command, ok := normalizeWidgetEscapeSequence(raw); ok {
		return command
	}
	if strings.TrimSpace(raw) == "" {
		if len(raw) > 0 {
			return "space"
		}
		return "enter"
	}
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "ctrl_c":
		return "q"
	case "backspace":
		return "b"
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
	case "pageup", "pgup", "pu":
		return "pgup"
	case "pagedown", "pgdown", "pgdn", "pd":
		return "pgdn"
	case "top", "begin", "first":
		return "top"
	case "end", "last", "bottom":
		return "end"
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

func normalizeWidgetEscapeSequence(raw string) (string, bool) {
	switch raw {
	case "\x1b[A":
		return "k", true
	case "\x1b[B":
		return "j", true
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
	case "\x1b[5~":
		return "pgup", true
	case "\x1b[6~":
		return "pgdn", true
	case "\x1b[Z", "\x1b[1;2Z":
		return "shift-tab", true
	case "\x1b[H", "\x1b[1~", "\x1b[7~":
		return "top", true
	case "\x1b[F", "\x1b[4~", "\x1b[8~":
		return "end", true
	default:
		return "", false
	}
}

func normalizeWidgetControlCommand(raw string, aliases map[string]string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return line
	}
	if strings.HasPrefix(line, ":") {
		line = strings.TrimSpace(strings.TrimPrefix(line, ":"))
	}
	normalized := normalizeWidgetCommand(line)
	if aliases != nil {
		if mapped, ok := aliases[normalized]; ok {
			return mapped
		}
	}
	return normalized
}

type widgetLoopControlAction int

const (
	widgetLoopControlNone widgetLoopControlAction = iota
	widgetLoopControlHandled
	widgetLoopControlQuit
)

type widgetLoopControlHandlers struct {
	QuitTokens    []string
	HelpTokens    []string
	RefreshTokens []string
	OnHelp        func()
	OnRefresh     func()
}

func handleWidgetLoopControlCommand(command string, handlers widgetLoopControlHandlers) widgetLoopControlAction {
	if widgetLoopCommandMatches(command, handlers.QuitTokens) {
		return widgetLoopControlQuit
	}
	if widgetLoopCommandMatches(command, handlers.HelpTokens) {
		if handlers.OnHelp != nil {
			handlers.OnHelp()
		}
		return widgetLoopControlHandled
	}
	if widgetLoopCommandMatches(command, handlers.RefreshTokens) {
		if handlers.OnRefresh != nil {
			handlers.OnRefresh()
		}
		return widgetLoopControlHandled
	}
	return widgetLoopControlNone
}

func widgetLoopCommandMatches(command string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(command))
	for _, token := range tokens {
		if normalized == strings.ToLower(strings.TrimSpace(token)) {
			return true
		}
	}
	return false
}
