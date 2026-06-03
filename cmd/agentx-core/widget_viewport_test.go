package main

import "testing"

func TestResolveWidgetPaneSize_UsesEnvironmentFallback(t *testing.T) {
	t.Setenv("LINES", "52")
	t.Setenv("COLUMNS", "118")

	height, width := resolveWidgetPaneSize()
	if height != 52 {
		t.Fatalf("expected height 52 from LINES, got %d", height)
	}
	if width != 118 {
		t.Fatalf("expected width 118 from COLUMNS, got %d", width)
	}
}

func TestResolveWidgetViewport_ComputesRowsAndCols(t *testing.T) {
	t.Setenv("LINES", "40")
	t.Setenv("COLUMNS", "90")

	rows, cols := resolveWidgetViewport(nil, 7, 2, false, 58, 20)
	if rows != 31 {
		t.Fatalf("expected rows 31, got %d", rows)
	}
	if cols != 86 {
		t.Fatalf("expected cols 86, got %d", cols)
	}

	rowsPrompt, colsPrompt := resolveWidgetViewport(nil, 7, 2, true, 58, 20)
	if rowsPrompt != 30 {
		t.Fatalf("expected prompt-mode rows 30, got %d", rowsPrompt)
	}
	if colsPrompt != 86 {
		t.Fatalf("expected prompt-mode cols 86, got %d", colsPrompt)
	}
}
