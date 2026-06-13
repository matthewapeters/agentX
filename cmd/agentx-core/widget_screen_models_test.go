package main

import "testing"

func TestNewInputWidgetScreenStateFromPane_ComputesViewportAndLayout(t *testing.T) {
	screen := NewInputWidgetScreenStateFromPane(24, 80, false)
	if screen.ViewportRows < 1 {
		t.Fatalf("expected positive viewport rows, got %d", screen.ViewportRows)
	}
	if screen.ViewportCols < 12 {
		t.Fatalf("expected viewport cols >= 12, got %d", screen.ViewportCols)
	}
	if screen.Layout.inputInnerTopRow <= 0 {
		t.Fatalf("expected positive input inner top row, got %d", screen.Layout.inputInnerTopRow)
	}
	if screen.Layout.controlInnerTopRow <= screen.Layout.inputInnerTopRow {
		t.Fatalf("expected control inner row below input row, got input=%d control=%d", screen.Layout.inputInnerTopRow, screen.Layout.controlInnerTopRow)
	}
	if screen.Components.ComposeBox.InnerTopRow != screen.Layout.inputInnerTopRow {
		t.Fatalf("expected compose-box anchor %d to match layout %d", screen.Components.ComposeBox.InnerTopRow, screen.Layout.inputInnerTopRow)
	}
	if screen.Components.ControlBox.InnerTopRow != screen.Layout.controlInnerTopRow {
		t.Fatalf("expected control-box anchor %d to match layout %d", screen.Components.ControlBox.InnerTopRow, screen.Layout.controlInnerTopRow)
	}
}

func TestNewInputWidgetScreenStateFromViewport_ClampsMinimums(t *testing.T) {
	screen := NewInputWidgetScreenStateFromViewport(0, 0, true)
	if screen.ViewportRows != 1 {
		t.Fatalf("expected viewport rows to clamp to 1, got %d", screen.ViewportRows)
	}
	if screen.ViewportCols != 12 {
		t.Fatalf("expected viewport cols to clamp to 12, got %d", screen.ViewportCols)
	}
}

func TestOutputWidgetScreenState_ContentBudgetHonorsMinimum(t *testing.T) {
	screen := NewOutputWidgetScreenState(18)
	budget := screen.ContentBudget("  │ [+] 🤖 Response: ")
	if budget < 12 {
		t.Fatalf("expected content budget >= 12, got %d", budget)
	}
	if screen.Components.Content.OuterPadding != 8 {
		t.Fatalf("expected output content outer padding 8, got %d", screen.Components.Content.OuterPadding)
	}
}

func TestNewInputWidgetScreenStateFromViewport_HelpRowsAreTracked(t *testing.T) {
	screen := NewInputWidgetScreenStateFromViewport(8, 40, true)
	if screen.Components.Header.HelpRows != 2 {
		t.Fatalf("expected help rows 2 when showHelp=true, got %d", screen.Components.Header.HelpRows)
	}
}
