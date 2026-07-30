package config

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"agentx/internal/llm/provider"
	"agentx/internal/state"
)

func TestBuildTree_nestedConfig(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
			},
			"llamacpp": map[string]any{
				"host": "localhost:8080",
				"model": "",
			},
		},
	}
	schema := map[string]provider.SchemaField{
		"agentx.provider":     {Name: "Provider", Type: "enum", RestartRequired: true},
		"agentx.ollama.host":  {Name: "Ollama Host", Type: "host", RestartRequired: true},
		"agentx.ollama.model": {Name: "Ollama Model", Type: "model", RestartRequired: true},
		"agentx.llamacpp.host": {Name: "llama.cpp Host", Type: "host", RestartRequired: true},
		"agentx.llamacpp.model": {Name: "llama.cpp Model", Type: "model", RestartRequired: true},
	}

	data := BuildTree(cfg, schema)

	// Should have sections: agentx (with provider), agentx.ollama, agentx.llamacpp
	if len(data.Sections) == 0 {
		t.Fatalf("expected sections, got empty tree")
	}

	// Find the agentx.ollama section
	var ollamaSection *section
	for i := range data.Sections {
		if data.Sections[i].name == "agentx.ollama" {
			ollamaSection = &data.Sections[i]
			break
		}
	}
	if ollamaSection == nil {
		t.Fatalf("expected agentx.ollama section, got sections: %v", sectionNames(data.Sections))
	}
	if len(ollamaSection.keys) != 2 {
		t.Fatalf("expected 2 keys in agentx.ollama, got %d", len(ollamaSection.keys))
	}
	if ollamaSection.keys[0].name != "host" {
		t.Fatalf("expected first key 'host', got %q", ollamaSection.keys[0].name)
	}
	if ollamaSection.keys[0].value != "localhost:11434" {
		t.Fatalf("expected value 'localhost:11434', got %q", ollamaSection.keys[0].value)
	}
}

func TestBuildTree_flatConfig(t *testing.T) {
	cfg := map[string]any{
		"provider": "ollama",
		"host":     "localhost:11434",
	}
	schema := map[string]provider.SchemaField{}

	data := BuildTree(cfg, schema)

	// Flat keys go under "agentx" section
	if len(data.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(data.Sections))
	}
	if data.Sections[0].name != "agentx" {
		t.Fatalf("expected section 'agentx', got %q", data.Sections[0].name)
	}
	if len(data.Sections[0].keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(data.Sections[0].keys))
	}
}

func TestBuildTree_emptyConfig(t *testing.T) {
	data := BuildTree(map[string]any{}, map[string]provider.SchemaField{})
	if len(data.Sections) != 0 {
		t.Fatalf("expected no sections for empty config, got %d", len(data.Sections))
	}
}

func TestBuildTree_schemaMetadata(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
		},
	}
	schema := map[string]provider.SchemaField{
		"agentx.provider": {
			Name:            "Provider",
			Type:            "enum",
			Description:     "The LLM backend.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
	}

	data := BuildTree(cfg, schema)
	if len(data.Sections) == 0 {
		t.Fatal("expected sections")
	}
	// Find agentx section
	var agentx *section
	for i := range data.Sections {
		if data.Sections[i].name == "agentx" {
			agentx = &data.Sections[i]
			break
		}
	}
	if agentx == nil {
		t.Fatal("expected agentx section")
	}
	if len(agentx.keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(agentx.keys))
	}
	k := agentx.keys[0]
	if k.label != "Provider" {
		t.Fatalf("expected label 'Provider', got %q", k.label)
	}
	if k.kind != "enum" {
		t.Fatalf("expected kind 'enum', got %q", k.kind)
	}
	if !k.restartRequired {
		t.Fatal("expected restart required")
	}
	if k.description != "The LLM backend." {
		t.Fatalf("expected description 'The LLM backend.', got %q", k.description)
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{nil, "(unset)"},
		{"hello", "hello"},
	}
	for _, tt := range tests {
		got := formatValue(tt.input)
		if got != tt.want {
			t.Errorf("formatValue(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConfigModel_RenderError(t *testing.T) {
	m := New(nil, "")
	m.Err = &transportError{"connection refused"}
	m.SetSize(80, 24)
	view := m.View()
	if !strings.Contains(view, "config read failed") {
		t.Fatalf("expected error message, got: %s", view)
	}
}

func TestConfigModel_RenderEmpty(t *testing.T) {
	m := New(nil, "")
	m.SetSize(80, 24)
	view := m.View()
	if !strings.Contains(view, "loading") {
		t.Fatalf("expected loading message for empty model, got: %s", view)
	}
}

func TestConfigModel_Navigation(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Initially on first key of first section (agentx has 1 key: provider)
	if m.Data.Selected != 0 {
		t.Fatalf("expected selected section 0, got %d", m.Data.Selected)
	}
	if m.Data.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.Data.Cursor)
	}

	// l moves to next section (agentx.ollama has 2 keys: host, model)
	m.Key(testKey("l"))
	if m.Data.Selected != 1 {
		t.Fatalf("expected section 1 after l, got %d", m.Data.Selected)
	}
	if m.Data.Cursor != 0 {
		t.Fatalf("expected cursor 0 after section nav, got %d", m.Data.Cursor)
	}

	// j moves cursor down within agentx.ollama
	m.Key(testKey("j"))
	if m.Data.Cursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.Data.Cursor)
	}

	// k moves cursor up
	m.Key(testKey("k"))
	if m.Data.Cursor != 0 {
		t.Fatalf("expected cursor 0 after k, got %d", m.Data.Cursor)
	}

	// Down past the end stays at last key (only 2 keys in this section)
	m.Key(testKey("j"))
	m.Key(testKey("j"))
	if m.Data.Cursor != 1 {
		t.Fatalf("expected cursor clamped to 1, got %d", m.Data.Cursor)
	}
}

func TestConfigModel_SectionNavigation(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"llamacpp": map[string]any{
				"host": "localhost:8080",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Initially on first section
	if m.Data.Selected != 0 {
		t.Fatalf("expected section 0, got %d", m.Data.Selected)
	}

	// l moves to next section
	m.Key(testKey("l"))
	if m.Data.Selected != 1 {
		t.Fatalf("expected section 1 after l, got %d", m.Data.Selected)
	}

	// h moves back
	m.Key(testKey("h"))
	if m.Data.Selected != 0 {
		t.Fatalf("expected section 0 after h, got %d", m.Data.Selected)
	}
}

func TestConfigModel_JumpTopBottom(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
				"api_key": "secret",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Move cursor to end
	m.Data.Cursor = 2

	// g jumps to top
	m.Key(testKey("g"))
	if m.Data.Cursor != 0 {
		t.Fatalf("expected cursor 0 after g, got %d", m.Data.Cursor)
	}

	// G jumps to bottom
	m.Key(testKey("G"))
	if m.Data.Cursor != 2 {
		t.Fatalf("expected cursor 2 after G, got %d", m.Data.Cursor)
	}
}

func TestConfigModel_CapturesKeys(t *testing.T) {
	m := New(nil, "")
	if m.CapturesKeys() {
		t.Fatal("read-only config surface should not capture keys")
	}
}

func TestConfigModel_SetSize(t *testing.T) {
	m := New(nil, "")
	m.SetSize(120, 40)
	if m.Width != 120 || m.Height != 40 {
		t.Fatalf("expected size 120x40, got %dx%d", m.Width, m.Height)
	}
}

func TestConfigModel_View(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")
	view := m.View()
	if !strings.Contains(view, "[agentx]") {
		t.Fatalf("expected [agentx] in view, got: %s", view)
	}
	if !strings.Contains(view, "j/k") {
		t.Fatalf("expected hint row in view, got: %s", view)
	}
}

// Phase 2b: Editing tests.

func TestConfigModel_EnterEdit(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Move to the key (ollama.host is in section "agentx.ollama")
	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	if ollamaIdx == -1 {
		t.Fatalf("expected agentx.ollama section, got: %v", sectionNames(m.Data.Sections))
	}
	m.Data.Selected = ollamaIdx
	m.Data.Cursor = 0 // host

	// Press Enter to enter edit mode
	m.Key(testKey("enter"))

	if m.Data.Edit == nil {
		t.Fatalf("expected edit mode to be active, got nil. SaveStatus=%s", m.Data.SaveStatus)
	}
	if m.Data.Edit.keyIndex != 0 {
		t.Fatalf("expected edit keyIndex 0, got %d", m.Data.Edit.keyIndex)
	}
	if m.Data.Edit.input != "localhost:11434" {
		t.Fatalf("expected initial input 'localhost:11434', got %q", m.Data.Edit.input)
	}
	if !m.CapturesKeys() {
		t.Fatal("expected CapturesKeys to return true in edit mode")
	}
}

func TestConfigModel_EditTypeText(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	m.Data.Selected = ollamaIdx
	m.Data.Cursor = 0
	m.Key(testKey("enter"))

	if m.Data.Edit == nil {
		t.Fatal("expected edit mode")
	}

	// Type a character
	m.Key(testKeyPress("a"))

	if m.Data.Edit.input != "localhost:11434a" {
		t.Fatalf("expected input 'localhost:11434a', got %q", m.Data.Edit.input)
	}
	if m.Data.Edit.cursor != 16 {
		t.Fatalf("expected cursor 16, got %d", m.Data.Edit.cursor)
	}
}

func TestConfigModel_EditBackspace(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	m.Data.Selected = ollamaIdx
	m.Data.Cursor = 0
	m.Key(testKey("enter"))

	if m.Data.Edit == nil {
		t.Fatal("expected edit mode")
	}

	// Type something
	m.Key(testKeyPress("x"))
	if m.Data.Edit.input != "localhost:11434x" {
		t.Fatalf("expected input with 'x', got %q", m.Data.Edit.input)
	}

	// Backspace
	m.Key(testKey("backspace"))
	if m.Data.Edit.input != "localhost:11434" {
		t.Fatalf("expected input without 'x', got %q", m.Data.Edit.input)
	}
	if m.Data.Edit.cursor != 15 {
		t.Fatalf("expected cursor 15 after backspace, got %d", m.Data.Edit.cursor)
	}
}

func TestConfigModel_EditEscape(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	m.Data.Selected = ollamaIdx
	m.Data.Cursor = 0
	m.Key(testKey("enter"))

	if m.Data.Edit == nil {
		t.Fatal("expected edit mode")
	}

	// Press Esc to cancel
	escMsg := testKey("escape")
	t.Logf("Esc key: String=%q", escMsg.String())
	m.Key(escMsg)

	if m.Data.Edit != nil {
		t.Fatalf("expected edit mode to be cancelled, got: %+v", m.Data.Edit)
	}
	if m.CapturesKeys() {
		t.Fatal("expected CapturesKeys to return false after escape")
	}
}

func TestConfigModel_ConfirmEdit(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	m.Data.Selected = ollamaIdx
	m.Data.Cursor = 0
	m.Key(testKey("enter"))

	// Confirm without changing
	m.Key(testKey("enter"))

	if m.Data.Edit != nil {
		t.Fatal("expected edit mode to be exited after confirm")
	}
	// Value should still be the same
	if m.Data.Sections[ollamaIdx].keys[0].value != "localhost:11434" {
		t.Fatalf("expected value unchanged, got %q", m.Data.Sections[ollamaIdx].keys[0].value)
	}
}

func TestConfigModel_ValidateInt(t *testing.T) {
	k := keyDef{kind: "int", minValue: 0, maxValue: 100}

	tests := []struct {
		input string
		valid bool
	}{
		{"42", true},
		{"0", true},
		{"-1", false},
		{"abc", false},
		{"101", false},
		{"", false},
	}

	for _, tt := range tests {
		err := validateValue("int", tt.input, k)
		if (err == nil) != tt.valid {
			t.Errorf("validateValue(%q) = %v, want valid=%v", tt.input, err, tt.valid)
		}
	}
}

func TestConfigModel_ValidateBool(t *testing.T) {
	k := keyDef{kind: "bool"}

	tests := []struct {
		input string
		valid bool
	}{
		{"true", true},
		{"false", true},
		{"yes", false},
		{"1", false},
	}

	for _, tt := range tests {
		err := validateValue("bool", tt.input, k)
		if (err == nil) != tt.valid {
			t.Errorf("validateValue(%q) = %v, want valid=%v", tt.input, err, tt.valid)
		}
	}
}

func TestConfigModel_ValidateEnum(t *testing.T) {
	k := keyDef{kind: "enum", enumerable: []string{"ollama", "llamacpp"}}

	tests := []struct {
		input string
		valid bool
	}{
		{"ollama", true},
		{"llamacpp", true},
		{"openai", false},
		{"", false},
	}

	for _, tt := range tests {
		err := validateValue("enum", tt.input, k)
		if (err == nil) != tt.valid {
			t.Errorf("validateValue(%q) = %v, want valid=%v", tt.input, err, tt.valid)
		}
	}
}

func TestConfigModel_ValidateString(t *testing.T) {
	k := keyDef{kind: "string"}

	tests := []struct {
		input string
		valid bool
	}{
		{"hello", true},
		{"  ", false},
		{"", false},
	}

	for _, tt := range tests {
		err := validateValue("string", tt.input, k)
		if (err == nil) != tt.valid {
			t.Errorf("validateValue(%q) = %v, want valid=%v", tt.input, err, tt.valid)
		}
	}
}

func TestConfigModel_ParseValue(t *testing.T) {
	tests := []struct {
		kind string
		input string
		want any
	}{
		{"bool", "true", true},
		{"bool", "false", false},
		{"int", "42", 42},
		{"int", "abc", "abc"},
		{"string", "hello", "hello"},
	}

	for _, tt := range tests {
		got := parseValue(tt.kind, tt.input)
		if got != tt.want {
			t.Errorf("parseValue(%q, %q) = %v, want %v", tt.kind, tt.input, got, tt.want)
		}
	}
}

func TestConfigModel_SerializeTree(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
			"provider": "ollama",
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Find the ollama section
	ollamaIdx := -1
	for i, sec := range m.Data.Sections {
		if sec.name == "agentx.ollama" {
			ollamaIdx = i
			break
		}
	}
	if ollamaIdx == -1 {
		t.Fatal("expected agentx.ollama section")
	}

	// Change a value
	m.Data.Sections[ollamaIdx].keys[0].value = "localhost:8080"

	// Serialize
	result := m.serializeTree()

	// Check the result
	ollamaCfg, ok := result["agentx.ollama"].(map[string]any)
	if !ok {
		t.Fatal("expected agentx.ollama section in result")
	}
	host, ok := ollamaCfg["host"].(string)
	if !ok || host != "localhost:8080" {
		t.Fatalf("expected host 'localhost:8080', got %v", host)
	}
}

// Helper functions.

// transportError is a simple error type for testing.
type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }

// sectionNames returns the section names for debugging.
func sectionNames(sections []section) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.name
	}
	return out
}

// testKey creates a KeyPressMsg from a key name or rune.
// Special keys like "enter", "escape", "backspace" use their constant codes.
func testKey(name string) tea.KeyPressMsg {
	// Check for special key names that map to constants.
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"}
	case "escape", "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace, Text: "backspace"}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome, Text: "home"}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd, Text: "end"}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	// For single-character keys, use the rune directly.
	if len(name) == 1 {
		r := rune(name[0])
		return tea.KeyPressMsg{Code: r, Text: name}
	}
	// Fallback: use the first rune.
	r := []rune(name)[0]
	return tea.KeyPressMsg{Code: r, Text: name}
}

// testKeyPress creates a KeyPressMsg for a printable character.
func testKeyPress(ch string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(ch[0]), Text: ch}
}

// --- Phase 3b: external change detection ---

func TestConfigModel_DiffSections_changedKey(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
			{name: "model", value: "llama3.1"},
		}},
	}
	new := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11435"},
			{name: "model", value: "llama3.1"},
		}},
	}
	changed := diffSections(old, new)
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed key, got %d: %v", len(changed), changed)
	}
	if changed[0] != "agentx.ollama.host" {
		t.Fatalf("expected changed key 'agentx.ollama.host', got %q", changed[0])
	}
}

func TestConfigModel_DiffSections_addedKey(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
	}
	new := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
			{name: "api_key", value: "secret"},
		}},
	}
	changed := diffSections(old, new)
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed key, got %d: %v", len(changed), changed)
	}
	if !strings.Contains(changed[0], "api_key") {
		t.Fatalf("expected added key to mention 'api_key', got %q", changed[0])
	}
}

func TestConfigModel_DiffSections_removedKey(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
			{name: "api_key", value: "secret"},
		}},
	}
	new := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
	}
	changed := diffSections(old, new)
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed key, got %d: %v", len(changed), changed)
	}
	if !strings.Contains(changed[0], "removed") {
		t.Fatalf("expected removed marker in %q", changed[0])
	}
}

func TestConfigModel_DiffSections_noChanges(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
	}
	new := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
	}
	changed := diffSections(old, new)
	if len(changed) != 0 {
		t.Fatalf("expected 0 changed keys, got %d: %v", len(changed), changed)
	}
}

func TestConfigModel_DiffSections_multipleSections(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
		{name: "agentx.theme", keys: []keyDef{
			{name: "active_border_color", value: "cyan"},
		}},
	}
	new := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
		{name: "agentx.theme", keys: []keyDef{
			{name: "active_border_color", value: "red"},
		}},
	}
	changed := diffSections(old, new)
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed key, got %d: %v", len(changed), changed)
	}
	if changed[0] != "agentx.theme.active_border_color" {
		t.Fatalf("expected 'agentx.theme.active_border_color', got %q", changed[0])
	}
}

func TestConfigModel_ExternalChange_RendersDialog(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Manually inject external change state.
	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}
	m.Data.Dialog = &dialogState{
		Kind:    dialogExternalFile,
		Title:   "File changed externally",
		Message: "agentx.toml was modified externally. Reload?",
		Options: []string{"Reload", "Keep changes", "Discard changes"},
		Selected: 0,
	}

	view := m.View()
	if !strings.Contains(view, "File changed externally") {
		t.Fatalf("expected dialog title in view, got: %s", view)
	}
	if !strings.Contains(view, "agentx.ollama.host") {
		t.Fatalf("expected changed key in view, got: %s", view)
	}
	if !strings.Contains(view, "Reload") {
		t.Fatalf("expected 'Reload' option in view, got: %s", view)
	}
}

func TestConfigModel_ExternalChange_RKey_Reload(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Inject external change state.
	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}

	// Press 'r' to reload. Without a transport client, reload reports an error
	// and the ExternalChange remains so the user can retry.
	m.Key(testKey("r"))

	if m.Data.SaveMsg != "reload failed: no transport" {
		t.Fatalf("expected transport error message, got: %q", m.Data.SaveMsg)
	}
	if m.Data.SaveStatus != SaveStateError {
		t.Fatalf("expected error status, got: %s", m.Data.SaveStatus)
	}
	// ExternalChange persists so the user can retry.
	if m.Data.ExternalChange == nil {
		t.Fatal("expected ExternalChange to persist for retry")
	}
}

func TestConfigModel_ExternalChange_Escape_Dismiss(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Inject external change state.
	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}
	m.Data.Dialog = &dialogState{
		Kind:    dialogExternalFile,
		Title:   "File changed externally",
		Message: "agentx.toml was modified externally. Reload?",
		Options: []string{"Reload", "Keep changes", "Discard changes"},
		Selected: 0,
	}

	// Press Escape to dismiss.
	m.Key(testKey("escape"))

	if m.Data.Dialog != nil {
		t.Fatalf("expected dialog to be dismissed, got: %+v", m.Data.Dialog)
	}
	if m.Data.ExternalChange != nil {
		t.Fatalf("expected ExternalChange to be nil after dismiss, got: %+v", m.Data.ExternalChange)
	}
}

func TestConfigModel_ExternalChange_KeepChanges(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}
	m.Data.Dialog = &dialogState{
		Kind:    dialogExternalFile,
		Title:   "File changed externally",
		Message: "agentx.toml was modified externally. Reload?",
		Options: []string{"Reload", "Keep changes", "Discard changes"},
		Selected: 1, // "Keep changes"
	}

	m.Key(testKey("enter"))

	if m.Data.Dialog != nil {
		t.Fatalf("expected dialog to be dismissed, got: %+v", m.Data.Dialog)
	}
	if m.Data.ExternalChange != nil {
		t.Fatalf("expected ExternalChange to be nil after keep, got: %+v", m.Data.ExternalChange)
	}
}

func TestConfigModel_ExternalChange_Debounce(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Inject external change state (simulates first event already handled).
	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}

	// Simulate a second config_changed event arriving while first is still pending.
	ev := state.Event{
		Epoch:       2000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	// Should not have created a new ExternalChange — the handler debounces.
	if m.Data.ExternalChange.NewHash != "def" {
		t.Fatalf("expected debounce to prevent double handling, got hash: %s", m.Data.ExternalChange.NewHash)
	}
}

func TestConfigModel_SnapshotSections_isolation(t *testing.T) {
	old := []section{
		{name: "agentx.ollama", keys: []keyDef{
			{name: "host", value: "localhost:11434"},
		}},
	}
	snap := snapshotSections(old)

	// Modify original.
	old[0].keys[0].value = "modified"

	// Snapshot should be unaffected.
	if snap[0].keys[0].value != "localhost:11434" {
		t.Fatalf("snapshot was modified: %s", snap[0].keys[0].value)
	}
}

func TestConfigModel_HintRow_ExternalChange(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}

	view := m.View()
	if !strings.Contains(view, "r reload") {
		t.Fatalf("expected 'r reload' in hint row, got: %s", view)
	}
}

func TestConfigModel_HintRow_BrowseMode_ExternalChange(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	m.Data.ExternalChange = &externalChangeState{
		Path:      "/tmp/agentx.toml",
		ChangedAt: 1000,
		OldHash:   "abc",
		NewHash:   "def",
		ChangedKeys: []string{"agentx.ollama.host"},
	}

	hint := m.hintRow()
	if !strings.Contains(hint, "r reload") {
		t.Fatalf("expected 'r reload' in hint row, got: %s", hint)
	}
	if !strings.Contains(hint, "esc dismiss") {
		t.Fatalf("expected 'esc dismiss' in hint row, got: %s", hint)
	}
	if !strings.Contains(hint, "1 keys changed") {
		t.Fatalf("expected '1 keys changed' in hint row, got: %s", hint)
	}
}

func TestConfigModel_HintRow_BrowseMode_Normal(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	hint := m.hintRow()
	// Normal browse mode should include 'r reload' as part of the hint.
	if !strings.Contains(hint, "r reload") {
		t.Fatalf("expected 'r reload' in normal hint row, got: %s", hint)
	}
}

// --- Phase 3c: conflict resolution + edge cases ---

// TestConfigModel_ExternalChange_ConflictResolution_TUIWins verifies that when
// the TUI has unsaved changes (SaveStatus == SaveStateUnsaved), an incoming
// external change event triggers a conflict-resolution dialog that defaults
// to "Keep TUI changes" (TUI wins).
func TestConfigModel_ExternalChange_ConflictResolution_TUIWins(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Put the TUI in unsaved state.
	m.Data.SaveStatus = SaveStateUnsaved
	m.Data.UnsavedChanges = true

	// Simulate an external change event.
	ev := state.Event{
		Epoch:       2000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	// Conflict-resolution dialog should be shown.
	if m.Data.Dialog == nil {
		t.Fatalf("expected dialog for conflict resolution, got nil")
	}
	if m.Data.Dialog.Kind != dialogExternalFile {
		t.Fatalf("expected dialogExternalFile, got %s", m.Data.Dialog.Kind)
	}
	if m.Data.Dialog.Title != "TUI changes take precedence" {
		t.Fatalf("expected title 'TUI changes take precedence', got %q", m.Data.Dialog.Title)
	}
	// First option (selected) should be "Keep TUI changes".
	if m.Data.Dialog.Selected != 0 {
		t.Fatalf("expected selected option 0, got %d", m.Data.Dialog.Selected)
	}
	if m.Data.Dialog.Options[0] != "Keep TUI changes" {
		t.Fatalf("expected first option 'Keep TUI changes', got %q", m.Data.Dialog.Options[0])
	}
	// Status bar should show the TUI-wins message.
	if !strings.Contains(m.Data.SaveMsg, "external change discarded") {
		t.Fatalf("expected status bar to mention 'external change discarded', got %q", m.Data.SaveMsg)
	}
	if !strings.Contains(m.Data.SaveMsg, "TUI changes take precedence") {
		t.Fatalf("expected status bar to mention 'TUI changes take precedence', got %q", m.Data.SaveMsg)
	}
	// ExternalChangeDiscarded flag should be set.
	if !m.Data.ExternalChangeDiscarded {
		t.Fatal("expected ExternalChangeDiscarded to be true")
	}
}

// TestConfigModel_ExternalChange_ConflictResolution_KeepTUIChanges verifies
// that selecting "Keep TUI changes" dismisses the dialog and clears
// ExternalChangeDiscarded.
func TestConfigModel_ExternalChange_ConflictResolution_KeepTUIChanges(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")
	m.Data.SaveStatus = SaveStateUnsaved

	ev := state.Event{
		Epoch:       2000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	// Confirm "Keep TUI changes".
	m.Key(testKey("enter"))

	if m.Data.Dialog != nil {
		t.Fatalf("expected dialog to be dismissed, got: %+v", m.Data.Dialog)
	}
	if m.Data.ExternalChange != nil {
		t.Fatalf("expected ExternalChange to be nil, got: %+v", m.Data.ExternalChange)
	}
	if m.Data.ExternalChangeDiscarded {
		t.Fatal("expected ExternalChangeDiscarded to be false after keeping TUI changes")
	}
	if !strings.Contains(m.Data.SaveMsg, "TUI changes take precedence") {
		t.Fatalf("expected status bar to mention 'TUI changes take precedence', got %q", m.Data.SaveMsg)
	}
}

// TestConfigModel_ExternalChange_ConflictResolution_Discard verifies that
// selecting "Discard" dismisses the dialog and clears the discarded flag.
func TestConfigModel_ExternalChange_ConflictResolution_Discard(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")
	m.Data.SaveStatus = SaveStateUnsaved

	ev := state.Event{
		Epoch:       2000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	// Select "Discard" (option 1).
	m.Data.Dialog.Selected = 1
	m.Key(testKey("enter"))

	if m.Data.Dialog != nil {
		t.Fatalf("expected dialog to be dismissed, got: %+v", m.Data.Dialog)
	}
	if m.Data.ExternalChange != nil {
		t.Fatalf("expected ExternalChange to be nil, got: %+v", m.Data.ExternalChange)
	}
	if m.Data.ExternalChangeDiscarded {
		t.Fatal("expected ExternalChangeDiscarded to be false")
	}
}

// TestConfigModel_ExternalChange_DebounceSurface verifies the surface-side
// debounce: a second event arriving within the debounce window coalesces with
// the first (timestamp is updated, no new dialog).
func TestConfigModel_ExternalChange_DebounceSurface(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Manually set LastExternalEventAt to simulate a previous event.
	m.Data.LastExternalEventAt = 1000

	// First new event — outside the debounce window (epoch 3000 vs 1000).
	ev1 := state.Event{
		Epoch:       3000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev1)

	// Should have processed the event.
	if m.Data.LastExternalEventAt != 3000 {
		t.Fatalf("expected LastExternalEventAt 3000, got %d", m.Data.LastExternalEventAt)
	}

	// Second event within debounce window (epoch 3000 + 500 = 3500, within 1s).
	ev2 := state.Event{
		Epoch:       3500,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev2)

	// The handler should coalesce: LastExternalEventAt updated to 3500,
	// but no new dialog or re-fetch triggered.
	if m.Data.LastExternalEventAt != 3500 {
		t.Fatalf("expected LastExternalEventAt 3500 after coalesce, got %d", m.Data.LastExternalEventAt)
	}
	// No second dialog should have been created.
	if m.Data.Dialog != nil {
		t.Fatalf("expected no new dialog from coalesced event, got: %+v", m.Data.Dialog)
	}
}

// TestConfigModel_ExternalChange_DebounceSurface_BeyondWindow verifies that an
// event arriving after the debounce window is processed normally.
func TestConfigModel_ExternalChange_DebounceSurface_BeyondWindow(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	m.Data.LastExternalEventAt = 1000

	// Event outside debounce window (> 1 second later).
	ev := state.Event{
		Epoch:       5000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	if m.Data.LastExternalEventAt != 5000 {
		t.Fatalf("expected LastExternalEventAt 5000, got %d", m.Data.LastExternalEventAt)
	}
}

// TestConfigModel_ExternalChange_QueueDuringSave verifies that when a
// config_changed event arrives while a save is in progress (SaveStatus ==
// SaveStateSaving), the event is queued in PendingExternalEvent instead of
// being processed immediately.
func TestConfigModel_ExternalChange_QueueDuringSave(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Simulate a save in progress.
	m.Data.SaveStatus = SaveStateSaving

	// External change event arrives during save.
	ev := state.Event{
		Epoch:       5000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)

	// Event should be queued.
	if m.Data.PendingExternalEvent == nil {
		t.Fatal("expected PendingExternalEvent to be set during save")
	}
	if m.Data.PendingExternalEvent.Epoch != 5000 {
		t.Fatalf("expected queued event epoch 5000, got %d", m.Data.PendingExternalEvent.Epoch)
	}
	// Status bar should indicate the queue.
	if !strings.Contains(m.Data.SaveMsg, "external change queued") {
		t.Fatalf("expected status bar to mention 'external change queued', got %q", m.Data.SaveMsg)
	}
}

// TestConfigModel_ProcessPendingExternalEvent verifies that processPendingExternalEvent
// processes the queued event and clears the queue.
func TestConfigModel_ProcessPendingExternalEvent(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Simulate a queued event.
	m.Data.PendingExternalEvent = &state.Event{
		Epoch:       5000,
		SessionID:   "test",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}

	// Process it.
	m.ProcessPendingExternalEvent()

	// Queue should be cleared.
	if m.Data.PendingExternalEvent != nil {
		t.Fatalf("expected PendingExternalEvent to be nil after processing, got: %+v", m.Data.PendingExternalEvent)
	}
	// LastExternalEventAt should be updated.
	if m.Data.LastExternalEventAt != 5000 {
		t.Fatalf("expected LastExternalEventAt 5000, got %d", m.Data.LastExternalEventAt)
	}
}

// TestConfigModel_ProcessPendingExternalEvent_NoEvent verifies that calling
// processPendingExternalEvent with no queued event is a no-op.
func TestConfigModel_ProcessPendingExternalEvent_NoEvent(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")

	// Should not panic or change state.
	m.ProcessPendingExternalEvent()

	if m.Data.PendingExternalEvent != nil {
		t.Fatal("expected PendingExternalEvent to remain nil")
	}
}

// TestConfigModel_ExternalChange_ConflictResolution_NoUnsavedChanges verifies
// that when the TUI has no unsaved changes, an external change via
// SimulateExternalChange (which bypasses FetchConfig) triggers the standard
// external change dialog (not the conflict-resolution dialog).
func TestConfigModel_ExternalChange_ConflictResolution_NoUnsavedChanges(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11434",
			},
		},
	}
	m := NewFromConfig(cfg, map[string]provider.SchemaField{}, 80, 24, nil, "")
	m.Data.SaveStatus = SaveStateSaved

	// Modify the config to simulate an external change.
	modified := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host": "localhost:11435",
			},
		},
	}

	// Use SimulateExternalChange to trigger the standard flow without a transport.
	err := m.SimulateExternalChange(modified, "/tmp/agentx.toml")
	if err != nil {
		t.Fatalf("SimulateExternalChange failed: %v", err)
	}

	// Should show the standard external change dialog (not the conflict one).
	if m.Data.Dialog == nil {
		t.Fatalf("expected dialog, got nil")
	}
	if m.Data.Dialog.Title != "File changed externally" {
		t.Fatalf("expected 'File changed externally' dialog, got %q", m.Data.Dialog.Title)
	}
	// Conflict resolution title should NOT appear.
	if m.Data.Dialog.Title == "TUI changes take precedence" {
		t.Fatal("should not show conflict resolution dialog when TUI has no unsaved changes")
	}
}
