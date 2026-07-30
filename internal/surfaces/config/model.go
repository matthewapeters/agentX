// Package config is the configuration surface (PD-CONFIG).
// Phase 2a: read-only tree render. Phase 2b: interactive editing with
// type-appropriate editors, auto-save, and inline validation.
//
// Launched with `agentx surface launch config`.
package config

// section holds the data for one config section (e.g. "[agentx.ollama]").
type section struct {
	name  string   // dotted path, e.g. "agentx.ollama"
	label string   // display name, e.g. "[agentx.ollama]"
	keys  []keyDef // ordered key definitions within this section
}

// keyDef holds the data for one config key within a section.
type keyDef struct {
	name            string   // dotted path within the section, e.g. "host"
	label           string   // display name (e.g. "Ollama Host")
	value           string   // current string value from the config payload
	kind            string   // type kind: "string", "int", "bool", "enum", "host", "model", "color"
	description     string   // human-readable description from the schema
	enumerable      []string // allowed enum values (nil if not an enum)
	restartRequired bool
	readOnly        bool
	minValue        int     // for int types: minimum allowed value
	maxValue        int     // for int types: maximum allowed value
	unit            string  // optional unit label (e.g. "s", "KiB")
}

// saveState enumerates the status bar states.
type saveState string

const (
	saveStateLoaded saveState = "loaded"
	saveStateSaved  saveState = "saved"
	saveStateUnsaved saveState = "unsaved"
	saveStateSaving saveState = "saving…"
	saveStateError  saveState = "error"
)

// editState captures the current edit session for one key.
type editState struct {
	keyIndex int    // index of the key being edited within its section
	input    string // current text input
	cursor   int    // cursor position within input
	error    string // validation error (empty when valid)
}

// dialogKind enumerates the modal dialogs the surface can show.
type dialogKind string

const (
	dialogNone         dialogKind = ""
	dialogConfirm      dialogKind = "confirm"
	dialogError        dialogKind = "error"
	dialogHelp         dialogKind = "help"
	dialogModelPick    dialogKind = "model_pick"
	dialogRestart      dialogKind = "restart"
	dialogExternalFile dialogKind = "external_file"
)

// dialogState captures an active modal dialog.
type dialogState struct {
	Kind     dialogKind
	Title    string
	Message  string
	Options  []string // "Restart now", "Restart later", "Discard changes", etc.
	Selected int      // which option is currently highlighted
	Source   string   // which key triggered the dialog (e.g. "agentx.provider")
}

// colorPickerState captures the active color-picker editing session.
type colorPickerState struct {
	keyIndex int          // index of the key being edited
	hexInput string       // current hex input (without leading #)
	nameInput string      // current named-color input
	palette  []colorSwatch // the named palette shown in the picker
	mode     string       // "hex", "name", "ansi" — active input mode
	error    string       // validation error
}

// colorSwatch represents one swatch in the named color palette.
type colorSwatch struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

// externalChangeState captures the state of a detected external file change.
// Set by Apply when a config_changed event arrives; consumed by the view to
// highlight changed keys and show the reload prompt.
type externalChangeState struct {
	Path      string   // path to the file that changed (for display)
	ChangedAt int64    // unix timestamp when the change was detected
	OldHash   string   // sha256 of the old config snapshot (for change detection)
	NewHash   string   // sha256 of the newly-fetched config
	ChangedKeys []string // dotted "section.key" paths whose values differ from the last known state
}

// modelPickerState captures the active model-picker editing session.
type modelPickerState struct {
	keyIndex int      // index of the key being edited
	Provider string   // the provider name ("ollama" or "llamacpp")
	Section  string   // the section name (e.g. "agentx.ollama")
	Options  []string // available models from the provider API
	Custom   string   // custom model name typed by the user
	Loading  bool     // true while waiting for provider models
	Error    string   // error if the provider is unreachable
	Selected int      // which model is currently highlighted
}

// modelData is the assembled tree the view renders, plus editing, dialog, and
// picker state. Phase 2c adds dialog and picker overlays on top of Phase 2a/2b.
// Phase 3b adds external change detection with diff highlighting.
type modelData struct {
	Sections      []section
	Selected      int          // index of the selected section
	Cursor        int          // index of the selected key within the section
	Expanded      map[int]bool // which sections are expanded (nil = all expanded)
	Edit          *editState   // non-nil when a free-text edit session is in progress
	SaveStatus    saveState
	SaveMsg       string
	ScrollOffset  int          // vertical scroll offset (first visible row index in flat row list)

	// Phase 2c: modal dialog overlay.
	Dialog *dialogState

	// Phase 2c: color picker overlay (replaces free-text edit for color keys).
	ColorPicker *colorPickerState

	// Phase 2c: model picker overlay (replaces free-text edit for model keys).
	ModelPicker *modelPickerState

	// Phase 2c: which keys have been changed and need restart.
	RestartKeys []string

	// Phase 2c: unsaved-changes indicator for quit confirmation.
	UnsavedChanges bool

	// Phase 3b: external file change detection.
	// ExternalChange is non-nil when an external editor modified agentx.toml
	// and the surface has re-fetched but not yet reconciled the diff.
	ExternalChange *externalChangeState

	// HighlightedKeys is a set of "section.key" dotted paths whose values
	// changed in the most recent external reload. Rendered with a yellow
	// background so the user can see what shifted.
	HighlightedKeys map[string]bool
}
