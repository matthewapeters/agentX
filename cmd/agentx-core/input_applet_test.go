package main

import (
	"strings"
	"testing"
)

func TestInputAppletRender_ClipsToTerminalHeight(t *testing.T) {
	applet := NewInputApplet()
	lines := applet.Render(8, 80)
	if len(lines) != 8 {
		t.Fatalf("expected 8 lines for height=8, got %d", len(lines))
	}
}

func TestInputAppletRender_NormalizesNonPositiveDimensions(t *testing.T) {
	applet := NewInputApplet()
	lines := applet.Render(0, 0)
	if len(lines) < 1 {
		t.Fatalf("expected at least one line for normalized dimensions, got %d", len(lines))
	}
}

func TestInputAppletRender_ScreenshotContract(t *testing.T) {
	applet := NewInputApplet()
	applet.compose.inputLines = [][]rune{[]rune("what is 2+2?")}
	applet.compose.cursorRow = 0
	applet.compose.cursorCol = len([]rune("what is 2+2?"))

	lines := applet.Render(8, 45)
	frame := strings.Join(lines, "\n")
	t.Logf("render lines (%d):\n%s", len(lines), frame)
	if strings.Contains(frame, "(2 lines truncated)") {
		t.Fatalf("expected no visible truncation sentinel in InputApplet render, got:\n%s", frame)
	}
}
