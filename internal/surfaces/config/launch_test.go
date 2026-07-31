// Package config — launch and initialization tests (PD-CONFIG).
//
// These tests verify that the config surface can be:
//   - Created with New (nil client for unit tests)
//   - Rendered (View) after initialization
//   - Loaded with config (NewFromConfig)
//   - Help overlay works (Phase 5)
//   - Color picker works
//   - Model picker works
//
// They complement the unit tests in config_test.go which cover the model
// logic (build tree, navigation, editing, validation, external change
// detection, etc.) but do not exercise the full launch lifecycle.
package config

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"agentx/internal/llm/provider"
)

// TestLaunch_NewModelCreatesValidState verifies that New returns a model that
// can be rendered and updated without panicking.
func TestLaunch_NewModelCreatesValidState(t *testing.T) {
	model := New(nil, "")
	if model == nil {
		t.Fatal("New returned nil")
	}

	if model.Width != 80 {
		t.Errorf("expected width 80, got %d", model.Width)
	}
	if model.Height != 24 {
		t.Errorf("expected height 24, got %d", model.Height)
	}
	if model.Data.Selected != -1 {
		t.Errorf("expected selected -1, got %d", model.Data.Selected)
	}
	if model.Data.Cursor != -1 {
		t.Errorf("expected cursor -1, got %d", model.Data.Cursor)
	}
}

// TestLaunch_RenderEmptyModel verifies that an empty model renders correctly.
func TestLaunch_RenderEmptyModel(t *testing.T) {
	model := New(nil, "")
	model.SetSize(80, 24)

	rendered := model.View()
	if !strings.Contains(rendered, "loading") {
		t.Error("expected loading message for empty model")
	}
}

// TestLaunch_LoadConfigWithTree verifies that NewFromConfig populates the
// model's sections from the schema.
func TestLaunch_LoadConfigWithTree(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
			},
		},
		"theme": map[string]any{
			"active_border_color": "cyan",
		},
	}

	schema := map[string]provider.SchemaField{
		"agentx.provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			Required:        true,
			ReadOnly:        false,
			Description:     "The LLM backend to use.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
		"agentx.ollama.host": {
			Name:            "Ollama Host",
			Type:            "host",
			Default:         "localhost:11434",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama host address.",
			RestartRequired: true,
		},
		"agentx.ollama.model": {
			Name:            "Ollama Model",
			Type:            "model",
			Default:         "",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama model name.",
			RestartRequired: true,
		},
		"theme.active_border_color": {
			Name:            "Active Border Color",
			Type:            "color",
			Default:         "cyan",
			Required:        false,
			ReadOnly:        false,
			Description:     "SGR foreground for the focused panel border.",
			RestartRequired: false,
		},
	}

	model := NewFromConfig(cfg, schema, 80, 24, nil, "")
	if model == nil {
		t.Fatal("NewFromConfig returned nil")
	}

	if len(model.Data.Sections) == 0 {
		t.Error("expected at least one section after loading config")
	}

	model.SetSize(80, 24)
	rendered := model.View()
	if rendered == "" {
		t.Error("expected non-empty render after loading config")
	}
}

// TestLaunch_HelpOverlay verifies that the help overlay can be opened (Phase 5).
func TestLaunch_HelpOverlay(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
		},
	}

	schema := map[string]provider.SchemaField{
		"agentx.provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			Required:        true,
			ReadOnly:        false,
			Description:     "The LLM backend to use.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
	}

	model := NewFromConfig(cfg, schema, 80, 24, nil, "")
	model.SetSize(80, 24)

	// Open help with '?' key.
	keyMsg := tea.KeyPressMsg(tea.Key{Code: '?', Text: "?"})
	model.Key(keyMsg)
	if model.Data.Dialog == nil {
		t.Error("expected dialog to be open after help key")
	}
	if model.Data.Dialog.Kind != dialogHelp {
		t.Errorf("expected help dialog, got %s", model.Data.Dialog.Kind)
	}
	// Verify KeyDocs is populated.
	if len(model.Data.Dialog.KeyDocs) == 0 {
		t.Error("expected KeyDocs to be populated in help dialog")
	}
}

// TestLaunch_ColorPicker verifies that color fields open the color picker.
func TestLaunch_ColorPicker(t *testing.T) {
	cfg := map[string]any{
		"theme": map[string]any{
			"active_border_color": "cyan",
		},
	}

	schema := map[string]provider.SchemaField{
		"theme.active_border_color": {
			Name:            "Active Border Color",
			Type:            "color",
			Default:         "cyan",
			Required:        false,
			ReadOnly:        false,
			Description:     "SGR foreground for the focused panel border.",
			RestartRequired: false,
		},
	}

	model := NewFromConfig(cfg, schema, 80, 24, nil, "")
	model.SetSize(80, 24)

	// Select the color key.
	model.Data.Selected = 0
	model.Data.Cursor = 0

	// Enter edit mode with Enter.
	model.Key(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.Data.ColorPicker == nil {
		t.Error("expected color picker to be open for color field")
	}

	// Exit color picker with Esc.
	model.Key(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if model.Data.ColorPicker != nil {
		t.Error("expected color picker to be closed after esc key")
	}
}

// TestLaunch_ModelPicker verifies that model fields open the model picker.
func TestLaunch_ModelPicker(t *testing.T) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"ollama": map[string]any{
				"host":  "localhost:11434",
				"model": "llama3.1",
			},
		},
	}

	schema := map[string]provider.SchemaField{
		"agentx.ollama.host": {
			Name:            "Ollama Host",
			Type:            "host",
			Default:         "localhost:11434",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama host address.",
			RestartRequired: true,
		},
		"agentx.ollama.model": {
			Name:            "Ollama Model",
			Type:            "model",
			Default:         "",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama model name.",
			RestartRequired: true,
		},
	}

	model := NewFromConfig(cfg, schema, 80, 24, nil, "")
	model.SetSize(80, 24)

	// Select the model key.
	model.Data.Selected = 0
	model.Data.Cursor = 1

	// Enter edit mode with Enter.
	model.Key(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.Data.ModelPicker == nil {
		t.Error("expected model picker to be open for model field")
	}

	// Exit model picker with Esc.
	model.Key(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if model.Data.ModelPicker != nil {
		t.Error("expected model picker to be closed after esc key")
	}
}
