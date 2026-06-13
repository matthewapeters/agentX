package main

import "strings"

func (c InputWidgetComponents) renderHeaderLines(showHelp bool) []string {
	lines := []string{"[INPUT]"}
	if showHelp {
		lines = append(lines,
			trimSingleLine("help: arrows move cursor | Shift+arrows pan view | Tab inserts tab", 96),
			trimSingleLine("control: ESC then :q quit | :? toggle help | Enter submit from control", 96),
		)
	}
	return lines
}

func (c InputWidgetComposeBoxComponent) render(color string, state *inputWidgetComposeState) []string {
	textRows := state.viewportRows
	textCols := state.viewportCols
	vOverflow := len(state.inputLines) > textRows
	maxLine := 0
	for _, line := range state.inputLines {
		if len(line) > maxLine {
			maxLine = len(line)
		}
	}
	hOverflow := maxLine > textCols
	trackRowStart, trackRowLen := scrollbarThumb(len(state.inputLines), textRows, state.viewRow, textRows)
	trackColStart, trackColLen := scrollbarThumb(maxLine, textCols, state.viewCol, textCols)

	top := color + "┌" + strings.Repeat("─", textCols+3) + "┐" + ansiReset
	rows := []string{top}
	for i := 0; i < textRows; i++ {
		lineIndex := state.viewRow + i
		content := state.renderInputViewportRow(lineIndex, textCols)
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
	rows = append(rows, color+"│ "+state.renderHorizontalScrollbar(textCols, hOverflow, trackColStart, trackColLen)+" │"+ansiReset)
	rows = append(rows, color+"└"+strings.Repeat("─", textCols+3)+"┘"+ansiReset)
	return rows
}

func (c InputWidgetControlBoxComponent) controlCols(fallback int) int {
	cols := c.InnerCols
	if cols < 16 {
		cols = fallback + 1
		if cols < 16 {
			cols = 16
		}
	}
	return cols
}

func (c InputWidgetControlBoxComponent) render(color string, state *inputWidgetComposeState) []string {
	cols := c.controlCols(state.viewportCols)
	content := state.renderControlContent(cols)
	return []string{
		color + "┌" + strings.Repeat("─", cols+2) + "┐" + ansiReset,
		color + "│ " + content + " │" + ansiReset,
		color + "└" + strings.Repeat("─", cols+2) + "┘" + ansiReset,
	}
}
