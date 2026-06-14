package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// OutputResponseFormatter defines response-specific formatting hooks for output widget rendering.
//
// Contract:
//   - width is a hard visible-width budget. Implementations must not assume callers can render
//     content wider than width without truncation.
//   - FormatCollapsedPreview should treat empty or whitespace-only raw content as empty preview
//     semantics used by the default path ("none").
//   - ANSI is allowed in returned strings only if implementations ensure visible-width stability;
//     the default formatter returns plain text consistent with existing wrapping/preview helpers.
//   - Both methods must be deterministic and idempotent for identical (raw, width) inputs.
type OutputResponseFormatter interface {
	FormatResponse(raw string, width int) []string
	FormatCollapsedPreview(raw string, width int) string
}

type plainTextOutputFormatter struct{}

type markdownOutputFormatter struct{}

var markdownStyleRunesRE = regexp.MustCompile(`[\*_~]`)
var markdownInlineCodeRE = regexp.MustCompile("`([^`]*)`")
var markdownLinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

func (f plainTextOutputFormatter) FormatResponse(raw string, width int) []string {
	return wrapOutputWidgetContent(raw, width)
}

func (f plainTextOutputFormatter) FormatCollapsedPreview(raw string, width int) string {
	return renderOutputWidgetCollapsedPreview(raw, width)
}

func (f markdownOutputFormatter) FormatResponse(raw string, width int) []string {
	normalized := normalizeMarkdownToTerminalLines(raw)
	if len(normalized) == 0 {
		return wrapOutputWidgetContent("", width)
	}
	parts := make([]string, 0, len(normalized))
	for _, line := range normalized {
		if strings.TrimSpace(line) == "" {
			parts = append(parts, "")
			continue
		}
		if strings.HasPrefix(line, "    ") {
			content := markdownInlineCodeRE.ReplaceAllString(strings.TrimSpace(line), "$1")
			if content == "" {
				parts = append(parts, "")
				continue
			}
			codeBudget := width - 4
			if codeBudget < 1 {
				codeBudget = 1
			}
			for _, wrapped := range wrapTextLines(content, codeBudget) {
				candidate := "    " + wrapped
				if visibleDisplayWidth(candidate) > width {
					candidate = renderTruncate(stripAnsi(candidate), width, "")
				}
				parts = append(parts, candidate)
			}
			continue
		}
		parts = append(parts, wrapOutputWidgetContent(line, width)...)
	}
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

func (f markdownOutputFormatter) FormatCollapsedPreview(raw string, width int) string {
	normalized := strings.Join(normalizeMarkdownToTerminalLines(raw), "\n")
	return renderOutputWidgetCollapsedPreview(normalized, width)
}

// PlainTextOutputFormatter returns the default formatter that preserves current plain-text behavior.
func PlainTextOutputFormatter() OutputResponseFormatter {
	return plainTextOutputFormatter{}
}

// MarkdownOutputFormatter returns a deterministic markdown-to-terminal formatter.
func MarkdownOutputFormatter() OutputResponseFormatter {
	return markdownOutputFormatter{}
}

func ResolveOutputResponseFormatterFromEnv() OutputResponseFormatter {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AGENTX_OUTPUT_FORMATTER")))
	if mode == "markdown" {
		return MarkdownOutputFormatter()
	}
	return PlainTextOutputFormatter()
}

// DefaultOutputResponseFormatter returns the default response formatter.
func DefaultOutputResponseFormatter() OutputResponseFormatter {
	return ResolveOutputResponseFormatterFromEnv()
}

func normalizeMarkdownToTerminalLines(raw string) []string {
	trimmed := strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.TrimSpace(trimmed) == "" {
		return []string{""}
	}
	lines := strings.Split(trimmed, "\n")
	normalized := make([]string, 0, len(lines))
	inCodeFence := false
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			if trimmedLine == "" {
				normalized = append(normalized, "")
				continue
			}
			normalized = append(normalized, "    "+trimmedLine)
			continue
		}
		normalized = append(normalized, normalizeMarkdownLine(trimmedLine))
	}
	if len(normalized) == 0 {
		return []string{""}
	}
	return normalized
}

func normalizeMarkdownLine(line string) string {
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "### ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "### "))
	} else if strings.HasPrefix(line, "## ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "## "))
	} else if strings.HasPrefix(line, "# ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "# "))
	}
	if len(line) > 1 {
		marker := line[0]
		if (marker == '-' || marker == '*' || marker == '+') && line[1] == ' ' {
			line = "- " + strings.TrimSpace(line[2:])
		}
	}
	line = markdownLinkRE.ReplaceAllString(line, "$1 ($2)")
	line = markdownInlineCodeRE.ReplaceAllString(line, "$1")
	line = markdownStyleRunesRE.ReplaceAllString(line, "")
	if orderedPrefix, ok := normalizeMarkdownOrderedPrefix(line); ok {
		return orderedPrefix
	}
	return strings.TrimSpace(line)
}

func normalizeMarkdownOrderedPrefix(line string) (string, bool) {
	if len(line) < 3 {
		return "", false
	}
	dot := strings.Index(line, ". ")
	if dot <= 0 {
		return "", false
	}
	number := line[:dot]
	if _, err := strconv.Atoi(number); err != nil {
		return "", false
	}
	body := strings.TrimSpace(line[dot+2:])
	if body == "" {
		return number + ".", true
	}
	return number + ". " + body, true
}

func maxOutputFormatterWidth(width int) int {
	if width < 12 {
		return 12
	}
	return width
}
