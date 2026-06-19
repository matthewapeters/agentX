package main

import "testing"

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
