package config

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// styles is the small palette used by the config surface.
var (
	sectionStyle      = lipgloss.NewStyle().Bold(true)
	keyLabelStyle     = lipgloss.NewStyle().Width(28)
	keyValueStyle     = lipgloss.NewStyle().Width(32)
	statusStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	restartStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	hintStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedBgStyle   = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	readOnlyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	editInputStyle    = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("15"))
	editCursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("21")).Foreground(lipgloss.Color("15"))
	saveStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	saveErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	saveSavingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dialogBgStyle     = lipgloss.NewStyle().Background(lipgloss.Color("235")).Padding(1, 2)
	dialogTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	dialogMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	dialogOptionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dialogOptionSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	helpLineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	colorSwatchStyle  = lipgloss.NewStyle().Width(12).Align(lipgloss.Center)
	// Phase 3b: yellow background for keys changed by an external editor.
	externalHighlightStyle = lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")) // yellow bg, black text
	externalLabelStyle   = lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0"))
	externalValueStyle   = lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0"))
)

// View renders the config tree: a header line, section list with expanded keys,
// and a hint/status footer line. Phase 2b adds edit mode, auto-save, and
// inline validation. Phase 2c adds dialog overlay, color picker, and model
// picker overlays.
func (m *ConfigModel) View() string {
	if m.Err != nil {
		return m.renderError()
	}
	if len(m.Data.Sections) == 0 {
		return m.renderEmpty()
	}

	// Phase 2c: if an overlay is active, render it on top of the tree.
	if m.Data.Dialog != nil || m.Data.ModelPicker != nil || m.Data.ColorPicker != nil {
		// Dim the tree behind the overlay.
		tree := m.renderTreeDimmed()
		overlay := m.renderOverlay()
		return tree + overlay
	}

	return m.renderTree()
}

// dimStyle renders text with reduced intensity for overlay backgrounds.
var dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// renderTreeDimmed renders the tree dimmed for overlay backgrounds.
func (m *ConfigModel) renderTreeDimmed() string {
	var b strings.Builder
	for i, sec := range m.Data.Sections {
		selected := i == m.Data.Selected
		if selected {
			line := dimStyle.Render(sectionStyle.Render(sec.label))
			b.WriteString(line + "\n")
		} else {
			line := dimStyle.Render(sectionStyle.Render(sec.label))
			b.WriteString(line + "\n")
		}

		for j, k := range sec.keys {
			rowSelected := selected && j == m.Data.Cursor
			if m.Data.Edit != nil && j == m.Data.Edit.keyIndex {
				line := dimStyle.Render(m.renderEditor(k, rowSelected))
				b.WriteString(line + "\n")
			} else {
				// Phase 3b: dimmed highlight for changed keys behind overlays.
				if m.Data.ExternalChange != nil && m.Data.HighlightedKeys != nil {
					dottedPath := sec.name + "." + k.name
					if m.Data.HighlightedKeys[dottedPath] {
						line := dimStyle.Render(externalHighlightStyle.Render(m.renderKeyRow(k, rowSelected)))
						b.WriteString(line + "\n")
						continue
					}
				}
				line := dimStyle.Render(m.renderKeyRow(k, rowSelected))
				b.WriteString(line + "\n")
			}
		}
	}
	b.WriteString(m.hintRow())
	return b.String()
}

// renderError shows the last transport read error.
func (m *ConfigModel) renderError() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render("✗ config read failed: " + m.Err.Error()))
	b.WriteByte('\n')
	b.WriteString(hintStyle.Render("restart the orchestrator and relaunch"))
	return b.String()
}

// renderEmpty shows a friendly "no config data" message when there is
// nothing to render yet.
func (m *ConfigModel) renderEmpty() string {
	return hintStyle.Render("loading configuration…")
}

// renderTree renders the full config tree for the current width/height.
func (m *ConfigModel) renderTree() string {
	var b strings.Builder

	// Section list + expanded keys.
	for i, sec := range m.Data.Sections {
		selected := i == m.Data.Selected
		if selected {
			b.WriteString(selectedBgStyle.Render(sectionStyle.Render(sec.label)) + "\n")
		} else {
			b.WriteString(sectionStyle.Render(sec.label) + "\n")
		}

		for j, k := range sec.keys {
			rowSelected := selected && j == m.Data.Cursor

			// If we're in edit mode and this is the edited key, render the editor.
			if m.Data.Edit != nil && j == m.Data.Edit.keyIndex {
				line := m.renderEditor(k, rowSelected)
				b.WriteString(line + "\n")
				// Show validation error below the editor.
				if m.Data.Edit.error != "" {
					b.WriteString("  " + errorStyle.Render("  ⚠ " + m.Data.Edit.error) + "\n")
				}
			} else {
				line := m.renderKeyRow(k, rowSelected)
				b.WriteString(line + "\n")
			}
		}
	}

	// Hint row at the bottom.
	b.WriteString(m.hintRow())
	return b.String()
}

// renderKeyRow renders one key's name/value/restart indicator as a single line.
func (m *ConfigModel) renderKeyRow(k keyDef, selected bool) string {
	label := keyLabelStyle.Render(k.label)
	value := keyValueStyle.Render(k.value)

	var suffix string
	if k.restartRequired {
		suffix = " " + restartStyle.Render("🔁 restart")
	}
	if k.readOnly {
		suffix = " " + readOnlyStyle.Render("(read-only)")
	}

	line := label + " " + value + suffix

	// Phase 3b: highlight changed keys with yellow background.
	dottedPath := m.activeSectionName() + "." + k.name
	if m.Data.HighlightedKeys != nil && m.Data.HighlightedKeys[dottedPath] {
		line = externalHighlightStyle.Render(line)
	} else if selected {
		line = selectedBgStyle.Render(line)
	}
	return line
}

// activeSectionName returns the dotted section name of the currently selected
// section, or "" if none is selected.
func (m *ConfigModel) activeSectionName() string {
	sec := m.activeSection()
	if sec == nil {
		return ""
	}
	return sec.name
}

// renderEditor renders the active edit widget for the given key.
func (m *ConfigModel) renderEditor(k keyDef, selected bool) string {
	edit := m.Data.Edit
	if edit == nil {
		return m.renderKeyRow(k, selected)
	}

	label := keyLabelStyle.Render(k.label)

	// Render the input field based on kind.
	var input string
	switch k.kind {
	case "bool":
		// Toggle switch: [true] or [false].
		if edit.input == "true" {
			input = "[✓ true ]"
		} else {
			input = "[✗ false]"
		}
	case "enum":
		// Dropdown-style: show selected value.
		input = edit.input
	case "host":
		// Host field with live test indicator.
		input = "🔗 " + edit.input + " [test]"
	case "color":
		// Color field shows the current value with picker hint.
		input = "🎨 " + edit.input + " [picker]"
	default:
		// Text input: show cursor position.
		if edit.cursor < len(edit.input) {
			before := edit.input[:edit.cursor]
			cursorChar := edit.input[edit.cursor]
			after := edit.input[edit.cursor+1:]
			input = before + editCursorStyle.Render(string(cursorChar)) + after
		} else {
			input = edit.input + "▌"
		}
	}

	value := editInputStyle.Render(input)
	suffix := ""
	if k.restartRequired {
		suffix = " " + restartStyle.Render("🔁 restart")
	}

	line := label + " " + value + suffix
	if selected {
		line = selectedBgStyle.Render(line)
	}
	return line
}

// hintRow returns the bottom hint/status line, showing save state and
// keybindings. Phase 2c extends with s (save/restart), ? (help), and overlay-
// specific hints.
func (m *ConfigModel) hintRow() string {
	var statusPart string

	switch m.Data.SaveStatus {
	case SaveStateLoaded:
		statusPart = saveStatusStyle.Render("● loaded")
	case SaveStateSaved:
		if m.Data.SaveMsg != "" {
			statusPart = saveStatusStyle.Render("● " + m.Data.SaveMsg)
		} else {
			statusPart = saveStatusStyle.Render("● saved")
		}
	case SaveStateSaving:
		statusPart = saveSavingStyle.Render("● saving…")
	case SaveStateError:
		if m.Data.SaveMsg != "" {
			statusPart = saveErrorStyle.Render("● " + m.Data.SaveMsg)
		} else {
			statusPart = saveErrorStyle.Render("● error")
		}
	case SaveStateUnsaved:
		statusPart = saveSavingStyle.Render("● unsaved")
	default:
		statusPart = hintStyle.Render("● idle")
	}

	// Add cursor position info.
	var navPart string
	sec := m.activeSection()
	if sec != nil {
		navPart = fmt.Sprintf("%s · [%s] key %d/%d",
			statusPart,
			sec.label,
			m.Data.Cursor+1,
			len(sec.keys),
		)
	} else {
		navPart = statusPart
	}

	// Phase 3b: external change hint takes priority when pending.
	if m.Data.ExternalChange != nil {
		return m.renderExternalChangeHint()
	}

	// Phase 2c: different hint rows for different overlays.
	if m.Data.Dialog != nil {
		return m.renderDialogHint()
	}
	if m.Data.ModelPicker != nil {
		return m.renderModelPickerHint()
	}
	if m.Data.ColorPicker != nil {
		return m.renderColorPickerHint()
	}

	// Add keybindings hint.
	hint := "j/k↑↓ scroll · h/l←→ sections · ↵ edit · s save · q quit · ? help · r reload"
	if m.Data.Edit != nil {
		switch m.activeSectionKey().kind {
		case "host":
			hint = "type host · ↵ test & save · esc cancel"
		case "model":
			hint = "type model name · ↵ accept · esc cancel"
		case "color":
			hint = "type color name/hex · ↵ accept · tab switch mode · esc cancel"
		default:
			hint = "type to edit · ↵ confirm · esc cancel · s save · q quit · ? help · r reload"
		}
	}

	var parts []string
	parts = append(parts, navPart)
	parts = append(parts, hint)
	return hintStyle.Render(strings.Join(parts, "  ·  "))
}

// activeSectionKey returns the currently selected keyDef, or a zero value.
func (m *ConfigModel) activeSectionKey() keyDef {
	sec := m.activeSection()
	if sec == nil || m.Data.Cursor >= len(sec.keys) {
		return keyDef{}
	}
	return sec.keys[m.Data.Cursor]
}

// renderDialogHint renders the hint row for an active dialog.
func (m *ConfigModel) renderDialogHint() string {
	dlg := m.Data.Dialog
	switch dlg.Kind {
	case dialogHelp:
		return hintStyle.Render("↵ close help · q quit")
	case dialogRestart:
		return hintStyle.Render("↑↓ select · ↵ confirm · esc cancel")
	case dialogConfirm:
		return hintStyle.Render("↑↓ select · ↵ confirm · esc cancel")
	case dialogError:
		return hintStyle.Render("↵ dismiss · q quit")
	default:
		return hintStyle.Render("↑↓ select · ↵ confirm · esc cancel")
	}
}

// renderModelPickerHint renders the hint row for the model picker.
func (m *ConfigModel) renderModelPickerHint() string {
	pk := m.Data.ModelPicker
	var info string
	if pk.Loading {
		info = "loading models…"
	} else if pk.Error != "" {
		info = "error: " + pk.Error + " · type custom name"
	} else if len(pk.Options) == 0 {
		info = "(no models) · type custom name"
	} else {
		info = fmt.Sprintf("%d models · ↑↓ select · ↵ accept · esc cancel", len(pk.Options))
	}
	return hintStyle.Render(info)
}

// renderColorPickerHint renders the hint row for the color picker.
func (m *ConfigModel) renderColorPickerHint() string {
	pk := m.Data.ColorPicker
	mode := pk.mode
	if mode == "" {
		mode = "name"
	}
	return hintStyle.Render(fmt.Sprintf(
		"%s mode · ↑↓ browse · tab cycle mode · ↵ accept · esc cancel",
		mode,
	))
}

// renderExternalChangeHint renders the hint row when an external change is pending.
// Phase 3b: AF-008.
func (m *ConfigModel) renderExternalChangeHint() string {
	ec := m.Data.ExternalChange
	var pathPart string
	if ec.Path != "" {
		pathPart = " (" + ec.Path + ")"
	}
	hint := fmt.Sprintf(
		"%d keys changed%s · r reload · ↑↓ select option · ↵ confirm · esc dismiss",
		len(ec.ChangedKeys), pathPart,
	)
	return hintStyle.Render(hint)
}

// renderOverlay renders a dialog or picker overlay on top of the tree. Called
// from View() when an overlay is active.
func (m *ConfigModel) renderOverlay() string {
	var b strings.Builder

	if m.Data.Dialog != nil {
		if m.Data.Dialog.Kind == dialogExternalFile {
			b.WriteString(m.renderExternalChangeOverlay())
			return b.String()
		}
		b.WriteString(m.renderDialogOverlay())
		return b.String()
	}
	if m.Data.ModelPicker != nil {
		b.WriteString(m.renderModelPickerOverlay())
		return b.String()
	}
	if m.Data.ColorPicker != nil {
		b.WriteString(m.renderColorPickerOverlay())
		return b.String()
	}
	return ""
}

// renderExternalChangeOverlay renders the external-file-change dialog.
// Phase 3b: AF-008.
func (m *ConfigModel) renderExternalChangeOverlay() string {
	dlg := m.Data.Dialog
	ec := m.Data.ExternalChange
	var b strings.Builder

	width := min(64, m.Width-4)
	if width < 30 {
		width = 30
	}

	b.WriteString("\n")
	b.WriteString(dialogTitleStyle.Render("┌" + strings.Repeat("─", width-2) + "┐") + "\n")

	// Title with file path. dlg.Title distinguishes the standard reload
	// prompt ("File changed externally") from the Phase 3c conflict
	// resolution dialog ("TUI changes take precedence") — both share
	// Kind == dialogExternalFile, so the title must come from the dialog
	// state, not be hardcoded.
	title := dlg.Title
	if ec != nil && ec.Path != "" {
		title += " · " + ec.Path
	}
	b.WriteString(dialogBgStyle.Render("│ " + dialogTitleStyle.Render(centerText(title, width-4)) + " ") + "\n")

	// Separator.
	b.WriteString(dialogBgStyle.Render("│" + strings.Repeat("─", width-2) + "│") + "\n")

	// Highlight the changed keys count.
	if ec != nil && len(ec.ChangedKeys) > 0 {
		countLine := fmt.Sprintf("%d key(s) changed:", len(ec.ChangedKeys))
		b.WriteString(dialogBgStyle.Render("│ " + hintStyle.Render(countLine) + strings.Repeat(" ", width-4-len(countLine)) + "│") + "\n")

		// Show up to 10 changed keys.
		visible := min(len(ec.ChangedKeys), 8)
		for i := 0; i < visible; i++ {
			key := ec.ChangedKeys[i]
			line := "  • " + key
			b.WriteString(dialogBgStyle.Render("│ " + externalLabelStyle.Render(line) + strings.Repeat(" ", width-4-len(line)-2) + "│") + "\n")
		}
		if len(ec.ChangedKeys) > visible {
			moreLine := fmt.Sprintf("  ... and %d more", len(ec.ChangedKeys)-visible)
			b.WriteString(dialogBgStyle.Render("│ " + hintStyle.Render(moreLine) + strings.Repeat(" ", width-4-len(moreLine)) + "│") + "\n")
		}
	} else {
		msgLine := "No key changes detected (refresh)."
		b.WriteString(dialogBgStyle.Render("│ " + dialogMessageStyle.Render(msgLine) + strings.Repeat(" ", width-4-len(msgLine)) + "│") + "\n")
	}

	// Separator.
	b.WriteString(dialogBgStyle.Render("│" + strings.Repeat("─", width-2) + "│") + "\n")

	// Options.
	for i, opt := range dlg.Options {
		style := dialogOptionStyle
		if i == dlg.Selected {
			style = dialogOptionSel
		}
		prefix := "  "
		if i == dlg.Selected {
			prefix = "▸ "
		}
		b.WriteString(dialogBgStyle.Render("│ "+prefix+style.Render(opt)+strings.Repeat(" ", width-4-len(opt)-len(prefix))+"│") + "\n")
	}

	// Fill remaining.
	for len(b.String()) < m.Height*2 {
		b.WriteString(dialogBgStyle.Render("│" + strings.Repeat(" ", width-2) + "│") + "\n")
	}

	b.WriteString(dialogTitleStyle.Render("└" + strings.Repeat("─", width-2) + "┘") + "\n")
	return b.String()
}

// renderDialogOverlay renders the modal dialog.
func (m *ConfigModel) renderDialogOverlay() string {
	dlg := m.Data.Dialog
	var b strings.Builder

	width := min(60, m.Width-4)
	if width < 20 {
		width = 20
	}

	b.WriteString("\n")
	b.WriteString(dialogTitleStyle.Render("┌" + strings.Repeat("─", width-2) + "┐") + "\n")
	b.WriteString(dialogBgStyle.Render("│ " + dialogTitleStyle.Render(centerText(dlg.Title, width-4)) + " " + dialogBgStyle.Render("│")) + "\n")
	b.WriteString(dialogBgStyle.Render("│" + strings.Repeat(" ", width-2) + "│") + "\n")

	// Phase 5: Help overlay with per-key documentation.
	if dlg.Kind == dialogHelp && len(dlg.KeyDocs) > 0 {
		renderHelpDocs(&b, dlg.KeyDocs, width)
	} else {
		// Message lines.
		lines := splitLines(dlg.Message)
		for i, line := range lines {
			if i == 0 {
				b.WriteString(dialogBgStyle.Render("│ " + dialogMessageStyle.Render(line) + " " + strings.Repeat(" ", width-4-len(line)) + "│") + "\n")
			} else {
				b.WriteString(dialogBgStyle.Render("│ " + strings.Repeat(" ", width-4) + "│") + "\n")
			}
		}
	}

	// Options.
	for i, opt := range dlg.Options {
		style := dialogOptionStyle
		if i == dlg.Selected {
			style = dialogOptionSel
		}
		prefix := "  "
		if i == dlg.Selected {
			prefix = "▸ "
		}
		b.WriteString(dialogBgStyle.Render("│ "+prefix+style.Render(opt)+strings.Repeat(" ", width-4-len(opt)-len(prefix))+"│") + "\n")
	}

	// Fill remaining rows.
	for len(b.String()) < m.Height*2 {
		b.WriteString(dialogBgStyle.Render("│" + strings.Repeat(" ", width-2) + "│") + "\n")
	}

	b.WriteString(dialogTitleStyle.Render("└" + strings.Repeat("─", width-2) + "┘") + "\n")
	return b.String()
}

// renderHelpDocs renders per-key help documentation in the help overlay.
func renderHelpDocs(b *strings.Builder, docs []keyHelpDoc, width int) {
	// Show up to 10 docs to avoid overflow.
	visible := min(len(docs), 10)
	for i := 0; i < visible; i++ {
		doc := docs[i]
		line := fmt.Sprintf("%-20s %s", doc.Label, doc.Description)
		if doc.RestartRequired {
			line += " 🔁"
		}
		if len(line) > width-4 {
			line = line[:width-4]
		}
		b.WriteString(dialogBgStyle.Render("│ " + helpLineStyle.Render(line) + " " + strings.Repeat(" ", width-4-len(line)) + "│") + "\n")
	}
}

// renderModelPickerOverlay renders the model picker.
func (m *ConfigModel) renderModelPickerOverlay() string {
	pk := m.Data.ModelPicker
	var b strings.Builder

	width := min(60, m.Width-4)
	if width < 20 {
		width = 20
	}

	b.WriteString("\n")
	b.WriteString(dialogTitleStyle.Render("┌" + strings.Repeat("─", width-2) + "┐") + "\n")
	title := "Model Picker"
	if pk.Provider != "" {
		title += " · " + pk.Provider
	}
	b.WriteString(dialogBgStyle.Render("│ " + dialogTitleStyle.Render(centerText(title, width-4)) + " ") + "\n")

	if pk.Loading {
		b.WriteString(dialogBgStyle.Render("│ " + saveSavingStyle.Render("Loading models…") + strings.Repeat(" ", width-4-14) + "│") + "\n")
	} else if pk.Error != "" {
		b.WriteString(dialogBgStyle.Render("│ " + errorStyle.Render("Error: "+pk.Error) + strings.Repeat(" ", width-4-len(pk.Error)-7) + "│") + "\n")
	}

	// Options list.
	visible := min(len(pk.Options), m.Height-8)
	for i := 0; i < visible; i++ {
		opt := pk.Options[i]
		style := dialogOptionStyle
		if i == pk.Selected {
			style = dialogOptionSel
		}
		prefix := "  "
		if i == pk.Selected {
			prefix = "▸ "
		}
		b.WriteString(dialogBgStyle.Render("│ "+prefix+style.Render(opt)+strings.Repeat(" ", width-4-len(opt)-len(prefix))+"│") + "\n")
	}

	// Custom input line.
	customLabel := "Custom:"
	if pk.Custom != "" {
		customLabel += " " + pk.Custom
	}
	b.WriteString(dialogBgStyle.Render("│ "+hintStyle.Render(customLabel)+strings.Repeat(" ", width-4-len(customLabel))+"│") + "\n")

	// Fill remaining.
	for len(b.String()) < m.Height*2 {
		b.WriteString(dialogBgStyle.Render("│" + strings.Repeat(" ", width-2) + "│") + "\n")
	}

	b.WriteString(dialogTitleStyle.Render("└" + strings.Repeat("─", width-2) + "┘") + "\n")
	return b.String()
}

// renderColorPickerOverlay renders the color picker.
func (m *ConfigModel) renderColorPickerOverlay() string {
	pk := m.Data.ColorPicker
	var b strings.Builder

	width := min(60, m.Width-4)
	if width < 20 {
		width = 20
	}

	b.WriteString("\n")
	b.WriteString(dialogTitleStyle.Render("┌" + strings.Repeat("─", width-2) + "┐") + "\n")
	b.WriteString(dialogBgStyle.Render("│ " + dialogTitleStyle.Render(centerText("Color Picker", width-4)) + " ") + "\n")

	// Input mode display.
	modeLabel := pk.mode
	if modeLabel == "" {
		modeLabel = "name"
	}
	modeHint := hintStyle.Render(fmt.Sprintf("mode: %s", modeLabel))
	b.WriteString(dialogBgStyle.Render("│ "+modeHint+strings.Repeat(" ", width-4-len(modeHint))+"│") + "\n")

	// Current value preview.
	current := m.colorPickerInput(pk)
	if current == "" {
		current = "(empty)"
	}
	currentLine := dialogMessageStyle.Render("value: " + current)
	b.WriteString(dialogBgStyle.Render("│ "+currentLine+strings.Repeat(" ", width-4-len(currentLine))+"│") + "\n")

	// Palette grid (name + hex).
	visible := min(len(pk.palette), m.Height-10)
	for i := 0; i < visible; i++ {
		sw := pk.palette[i]
		line := fmt.Sprintf("  %s  %s", sw.Name, sw.Hex)
		b.WriteString(dialogBgStyle.Render("│ " + helpLineStyle.Render(line) + strings.Repeat(" ", width-4-len(line))+"│") + "\n")
	}

	// Fill remaining.
	for len(b.String()) < m.Height*2 {
		b.WriteString(dialogBgStyle.Render("│" + strings.Repeat(" ", width-2) + "│") + "\n")
	}

	b.WriteString(dialogTitleStyle.Render("└" + strings.Repeat("─", width-2) + "┘") + "\n")
	return b.String()
}

// --- helpers ---

// min returns the smaller of a or b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// splitLines splits s by newlines, returning at least one element.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

// centerText centers text within a width, padding with spaces.
func centerText(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}
