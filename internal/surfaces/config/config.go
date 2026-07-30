package config

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/llm/provider"
	"agentx/internal/state"
	transporthttp "agentx/internal/transport/http"
)

// ConfigModel renders the current effective configuration as a navigable tree.
// It fetches GET /config and GET /config/schema from the orchestrator and
// projects them into sections keyed by their dotted path (e.g. "agentx.ollama"),
// each with a list of keys showing name/value/type metadata. Editing,
// auto-save, and inline validation are implemented in Phase 2b.
//
// Launched with `agentx surface launch config`.
type ConfigModel struct {
	Data modelData

	Width  int
	Height int

	// Err holds the last transport read error (nil when no error has occurred).
	Err error

	// schema is kept for type hints during rendering.
	schema map[string]provider.SchemaField

	// client is the transport client for POST /config and POST /test/host.
	client *transporthttp.Client
	// token is the attach token for authorized requests.
	token string
}

// New returns a config model sized to 80×24 by default, bound to the given
// transport client and token. Call Fetch (or let the surface host drive it)
// before rendering.
func New(client *transporthttp.Client, token string) *ConfigModel {
	return &ConfigModel{
		Width:  80,
		Height: 24,
		client: client,
		token:  token,
		Data: modelData{
			Selected: -1,
			Cursor:   -1,
			Expanded: make(map[int]bool),
		},
	}
}

// NewFromConfig returns a config model pre-loaded with the given config and
// schema. Primarily useful for tests — the production path uses New + Fetch.
func NewFromConfig(cfg map[string]any, schema map[string]provider.SchemaField, width, height int, client *transporthttp.Client, token string) *ConfigModel {
	return &ConfigModel{
		Width:  width,
		Height: height,
		client: client,
		token:  token,
		Data: modelData{
			Sections: BuildTree(cfg, schema).Sections,
			Selected: 0,
			Cursor:   0,
			Expanded: make(map[int]bool),
		},
	}
}

// FetchConfig fetches the current effective config and schema from the
// orchestrator, merging them into the model's internal tree. Returns nil on
// success, or an error if either read fails.
func (m *ConfigModel) FetchConfig() error {
	if m.client == nil {
		m.Err = fmt.Errorf("no transport client")
		return m.Err
	}
	cfg, schema, err := m.client.FetchConfigSchema()
	if err != nil {
		m.Err = err
		m.Data = modelData{Selected: -1, Cursor: -1, Expanded: m.Data.Expanded}
		return err
	}
	m.Err = nil
	m.schema = schema
	m.Data = BuildTree(cfg, schema)
	m.Data.Selected = 0
	m.Data.Cursor = 0
	m.Data.SaveStatus = SaveStateLoaded
	return nil
}

// LoadConfigDirect loads config and schema directly into the model without a
// transport client. Primarily used for tests and simulated external-change
// detection where the orchestrator is not available.
func (m *ConfigModel) LoadConfigDirect(cfg map[string]any, schema map[string]provider.SchemaField) {
	m.schema = schema
	m.Data = BuildTree(cfg, schema)
	m.Data.Selected = 0
	m.Data.Cursor = 0
	m.Data.SaveStatus = SaveStateLoaded
	m.Err = nil
}

// SimulateExternalChange is a test helper that loads a modified config map,
// diffs it against the model's current state, populates HighlightedKeys,
// and shows the external-change dialog. It replaces the transport-dependent
// fetch+diff pipeline for testing purposes.
func (m *ConfigModel) SimulateExternalChange(modifiedCfg map[string]any, path string) error {
	oldSections := snapshotSections(m.Data.Sections)
	oldHash := configHash(m.Data.Sections)

	m.LoadConfigDirect(modifiedCfg, m.schema)

	newHash := configHash(m.Data.Sections)

	var changedKeys []string
	if oldHash != newHash {
		changedKeys = diffSections(oldSections, m.Data.Sections)
	}

	m.Data.ExternalChange = &externalChangeState{
		Path:        path,
		ChangedAt:   0,
		OldHash:     oldHash,
		NewHash:     newHash,
		ChangedKeys: changedKeys,
	}
	m.Data.HighlightedKeys = make(map[string]bool, len(changedKeys))
	for _, k := range changedKeys {
		m.Data.HighlightedKeys[k] = true
	}

	m.Data.Dialog = &dialogState{
		Kind:     dialogExternalFile,
		Title:    "File changed externally",
		Message:  "agentx.toml was modified externally. Reload?",
		Options:  []string{"Reload", "Keep changes", "Discard changes"},
		Selected: 0,
		Source:   path,
	}
	m.Data.SaveStatus = SaveStateSaved
	m.Data.SaveMsg = fmt.Sprintf("external change detected (%d keys)", len(changedKeys))
	return nil
}

// Error returns the last transport read error, or nil if no error has occurred.
func (m *ConfigModel) Error() error { return m.Err }

// BuildTree merges the config payload with the schema to produce a navigable
// tree. The config payload is keyed by section dotted path (e.g. "agentx.ollama")
// mapping to a map of key→value. The schema maps dotted key paths to their
// metadata (type, description, restart_required).
func BuildTree(cfg map[string]any, schema map[string]provider.SchemaField) modelData {
	// 1. Group config keys by their section prefix (the longest dotted prefix
	//    that appears as a top-level key in cfg).
	sectionKeys := groupBySection(cfg)

	// 2. Build section list, ordered deterministically by section name.
	var sections []section
	sectionNames := make([]string, 0, len(sectionKeys))
	for s := range sectionKeys {
		sectionNames = append(sectionNames, s)
	}
	sort.Strings(sectionNames)

	for _, secName := range sectionNames {
		keys := buildSectionKeys(sectionKeys[secName], secName, schema)
		sections = append(sections, section{
			name:  secName,
			label: fmt.Sprintf("[%s]", secName),
			keys:  keys,
		})
	}

	return modelData{
		Sections: sections,
		Selected: 0,
		Cursor:   0,
		Expanded: make(map[int]bool),
	}
}

// groupBySection splits cfg into per-section maps. Top-level keys with no dots
// become the section "agentx"; dotted keys (e.g. "agentx.ollama.host") are
// folded under their section prefix (e.g. "agentx.ollama").
func groupBySection(cfg map[string]any) map[string]map[string]any {
	sections := make(map[string]map[string]any)
	for key, val := range cfg {
		// If the value is a nested map, it is itself a section — flatten its
		// entries with the prefix so the tree has one level per dotted segment.
		if nested, ok := val.(map[string]any); ok {
			secName := key
			for nk, nv := range nested {
				full := secName + "." + nk
				if sub, ok := nv.(map[string]any); ok {
					// Two-level nesting: fold under the deeper section.
					sections[full] = mergeMap(sections[full], sub)
				} else {
					if sections[secName] == nil {
						sections[secName] = make(map[string]any)
					}
					sections[secName][nk] = nv
				}
			}
			continue
		}
		// Scalar top-level key — lives under the root section "agentx".
		if sections["agentx"] == nil {
			sections["agentx"] = make(map[string]any)
		}
		sections["agentx"][key] = val
	}
	return sections
}

// mergeMap copies all entries from src into dst, overwriting on conflict.
func mergeMap(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any)
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// buildSectionKeys constructs the key list for one section, pulling schema
// metadata where available.
func buildSectionKeys(kv map[string]any, sectionName string, schema map[string]provider.SchemaField) []keyDef {
	keys := make([]keyDef, 0, len(kv))
	// Sort key names for stable render.
	names := make([]string, 0, len(kv))
	for n := range kv {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		val := kv[name]
		sField, hasSchema := schema[sectionName+"."+name]

		kind := inferKind(val)
		if hasSchema && sField.Type != "" {
			kind = sField.Type
		}

		var enumVals []string
		if hasSchema && len(sField.EnumValues) > 0 {
			enumVals = sField.EnumValues
		}

		// For enum schemas with no explicit type set, use "enum" as the kind.
		if hasSchema && sField.Type == "" && len(sField.EnumValues) > 0 {
			kind = "enum"
		}

		keys = append(keys, keyDef{
			name:            name,
			label:           fieldName(sField, name),
			value:           formatValue(val),
			kind:            kind,
			description:     sField.Description,
			enumerable:      enumVals,
			restartRequired: sField.RestartRequired,
			readOnly:        sField.ReadOnly,
			minValue:        0,
			maxValue:        0,
		})
	}
	return keys
}

// fieldName returns the schema's display name if present, falling back to the
// dotted key path formatted with spaces.
func fieldName(f provider.SchemaField, name string) string {
	if f.Name != "" {
		return f.Name
	}
	return strings.Join(strings.Split(name, "_"), " ")
}

// inferKind guesses a value's kind from its Go type.
func inferKind(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int, int32, int64, float32, float64:
		return "int"
	case string:
		return "string"
	default:
		return "string"
	}
}

// formatValue renders a config value as a display string.
func formatValue(v any) string {
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float32:
		if val == float32(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int, int32, int64:
		return fmt.Sprintf("%d", val)
	case nil:
		return "(unset)"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// surfaceDebounceWindowSec is the surface-side debounce window (in seconds)
// for external change detection. Events arriving within this window of the
// previous event are coalesced into a single notification. Matches the
// orchestrator-side debounceWindow (100ms).
const surfaceDebounceWindowSec = 1

// colorPalette is the built-in named-color palette for the color picker.
var colorPalette = []colorSwatch{
	{Name: "black", Hex: "#000000"},
	{Name: "maroon", Hex: "#800000"},
	{Name: "green", Hex: "#008000"},
	{Name: "olive", Hex: "#808000"},
	{Name: "navy", Hex: "#000080"},
	{Name: "purple", Hex: "#800080"},
	{Name: "teal", Hex: "#008080"},
	{Name: "silver", Hex: "#c0c0c0"},
	{Name: "gray", Hex: "#808080"},
	{Name: "red", Hex: "#ff0000"},
	{Name: "lime", Hex: "#00ff00"},
	{Name: "yellow", Hex: "#ffff00"},
	{Name: "blue", Hex: "#0000ff"},
	{Name: "fuchsia", Hex: "#ff00ff"},
	{Name: "aqua", Hex: "#00ffff"},
	{Name: "white", Hex: "#ffffff"},
	{Name: "alice blue", Hex: "#f0f8ff"},
	{Name: "alice-blue", Hex: "#f0f8ff"},
	{Name: "aliceblue", Hex: "#f0f8ff"},
	{Name: "cornflower blue", Hex: "#6495ed"},
	{Name: "cornflower-blue", Hex: "#6495ed"},
	{Name: "cornflowerblue", Hex: "#6495ed"},
	{Name: "cyan", Hex: "#00ffff"},
	{Name: "dark cyan", Hex: "#008b8b"},
	{Name: "dark-cyan", Hex: "#008b8b"},
	{Name: "darkcyan", Hex: "#008b8b"},
	{Name: "dark gray", Hex: "#a9a9a9"},
	{Name: "dark-gray", Hex: "#a9a9a9"},
	{Name: "darkgrey", Hex: "#a9a9a9"},
	{Name: "darkgray", Hex: "#a9a9a9"},
	{Name: "dark green", Hex: "#006400"},
	{Name: "dark-green", Hex: "#006400"},
	{Name: "darkgreen", Hex: "#006400"},
	{Name: "dark khaki", Hex: "#bdb76b"},
	{Name: "dark-khaki", Hex: "#bdb76b"},
	{Name: "darkkhaki", Hex: "#bdb76b"},
	{Name: "dark magenta", Hex: "#8b008b"},
	{Name: "dark-magenta", Hex: "#8b008b"},
	{Name: "darkmagenta", Hex: "#8b008b"},
	{Name: "dark olive green", Hex: "#556b2f"},
	{Name: "dark-olive-green", Hex: "#556b2f"},
	{Name: "darkolivegreen", Hex: "#556b2f"},
	{Name: "dark orange", Hex: "#ff8c00"},
	{Name: "dark-orange", Hex: "#ff8c00"},
	{Name: "darkorange", Hex: "#ff8c00"},
	{Name: "dark orchid", Hex: "#9932cc"},
	{Name: "dark-orchid", Hex: "#9932cc"},
	{Name: "darkorchid", Hex: "#9932cc"},
	{Name: "dark red", Hex: "#8b0000"},
	{Name: "dark-red", Hex: "#8b0000"},
	{Name: "darkred", Hex: "#8b0000"},
	{Name: "dark salmon", Hex: "#e9967a"},
	{Name: "dark-salmon", Hex: "#e9967a"},
	{Name: "darksalmon", Hex: "#e9967a"},
	{Name: "dark sea green", Hex: "#8fbc8f"},
	{Name: "dark-sea-green", Hex: "#8fbc8f"},
	{Name: "darkseagreen", Hex: "#8fbc8f"},
	{Name: "dark slate blue", Hex: "#483d8b"},
	{Name: "dark-slate-blue", Hex: "#483d8b"},
	{Name: "darkslateblue", Hex: "#483d8b"},
	{Name: "dark slate gray", Hex: "#2f4f4f"},
	{Name: "dark-slate-gray", Hex: "#2f4f4f"},
	{Name: "dark-slate-grey", Hex: "#2f4f4f"},
	{Name: "darkslategray", Hex: "#2f4f4f"},
	{Name: "darkslategrey", Hex: "#2f4f4f"},
	{Name: "dark turquoise", Hex: "#00ced1"},
	{Name: "dark-turquoise", Hex: "#00ced1"},
	{Name: "darkturquoise", Hex: "#00ced1"},
	{Name: "dark violet", Hex: "#9400d3"},
	{Name: "dark-violet", Hex: "#9400d3"},
	{Name: "darkviolet", Hex: "#9400d3"},
	{Name: "deep pink", Hex: "#ff1493"},
	{Name: "deep-pink", Hex: "#ff1493"},
	{Name: "deeppink", Hex: "#ff1493"},
	{Name: "deep sky blue", Hex: "#00bfff"},
	{Name: "deep-sky-blue", Hex: "#00bfff"},
	{Name: "deepskyblue", Hex: "#00bfff"},
	{Name: "dodger blue", Hex: "#1e90ff"},
	{Name: "dodger-blue", Hex: "#1e90ff"},
	{Name: "dodgerblue", Hex: "#1e90ff"},
	{Name: "fire brick", Hex: "#b22222"},
	{Name: "fire-brick", Hex: "#b22222"},
	{Name: "firebrick", Hex: "#b22222"},
	{Name: "floral white", Hex: "#fffaf0"},
	{Name: "floral-white", Hex: "#fffaf0"},
	{Name: "floralwhite", Hex: "#fffaf0"},
	{Name: "forest green", Hex: "#228b22"},
	{Name: "forest-green", Hex: "#228b22"},
	{Name: "forestgreen", Hex: "#228b22"},
	{Name: "ghost white", Hex: "#f8f8ff"},
	{Name: "ghost-white", Hex: "#f8f8ff"},
	{Name: "ghostwhite", Hex: "#f8f8ff"},
	{Name: "goldenrod", Hex: "#daa520"},
	{Name: "gold", Hex: "#ffd700"},
	{Name: "golden", Hex: "#ffd700"},
	{Name: "goldensrod", Hex: "#daa520"},
	{Name: "gainsboro", Hex: "#dcdcdc"},
	{Name: "green yellow", Hex: "#adff2f"},
	{Name: "green-yellow", Hex: "#adff2f"},
	{Name: "greenyellow", Hex: "#adff2f"},
	{Name: "hot pink", Hex: "#ff69b4"},
	{Name: "hot-pink", Hex: "#ff69b4"},
	{Name: "hotpink", Hex: "#ff69b4"},
	{Name: "indian red", Hex: "#cd5c5c"},
	{Name: "indian-red", Hex: "#cd5c5c"},
	{Name: "indianred", Hex: "#cd5c5c"},
	{Name: "indigo", Hex: "#4b0082"},
	{Name: "ivory", Hex: "#fffff0"},
	{Name: "khaki", Hex: "#f0e68c"},
	{Name: "lavender", Hex: "#e6e6fa"},
	{Name: "lavender blush", Hex: "#fff0f5"},
	{Name: "lavender-blush", Hex: "#fff0f5"},
	{Name: "lavenderblush", Hex: "#fff0f5"},
	{Name: "lawngreen", Hex: "#7cfc00"},
	{Name: "light blue", Hex: "#add8e6"},
	{Name: "light-blue", Hex: "#add8e6"},
	{Name: "lightblue", Hex: "#add8e6"},
	{Name: "light coral", Hex: "#f08080"},
	{Name: "light-coral", Hex: "#f08080"},
	{Name: "lightcoral", Hex: "#f08080"},
	{Name: "light cyan", Hex: "#e0ffff"},
	{Name: "light-cyan", Hex: "#e0ffff"},
	{Name: "lightcyan", Hex: "#e0ffff"},
	{Name: "light goldenrod", Hex: "#fafad2"},
	{Name: "light-goldenrod", Hex: "#fafad2"},
	{Name: "lightgoldenrodyellow", Hex: "#fafad2"},
	{Name: "light gray", Hex: "#d3d3d3"},
	{Name: "light-gray", Hex: "#d3d3d3"},
	{Name: "lightgrey", Hex: "#d3d3d3"},
	{Name: "lightgray", Hex: "#d3d3d3"},
	{Name: "light green", Hex: "#90ee90"},
	{Name: "light-green", Hex: "#90ee90"},
	{Name: "lightgreen", Hex: "#90ee90"},
	{Name: "light pink", Hex: "#ffb6c1"},
	{Name: "light-pink", Hex: "#ffb6c1"},
	{Name: "lightpink", Hex: "#ffb6c1"},
	{Name: "light salmon", Hex: "#ffa07a"},
	{Name: "light-salmon", Hex: "#ffa07a"},
	{Name: "lightsalmon", Hex: "#ffa07a"},
	{Name: "light sea green", Hex: "#20b2aa"},
	{Name: "light-sea-green", Hex: "#20b2aa"},
	{Name: "lightseagreen", Hex: "#20b2aa"},
	{Name: "light sky blue", Hex: "#87cefa"},
	{Name: "light-sky-blue", Hex: "#87cefa"},
	{Name: "lightskyblue", Hex: "#87cefa"},
	{Name: "light slate gray", Hex: "#778899"},
	{Name: "light-slate-gray", Hex: "#778899"},
	{Name: "light-slate-grey", Hex: "#778899"},
	{Name: "lightsteelblue", Hex: "#b0c4de"},
	{Name: "light yellow", Hex: "#ffffe0"},
	{Name: "light-yellow", Hex: "#ffffe0"},
	{Name: "lightyellow", Hex: "#ffffe0"},
	{Name: "lime green", Hex: "#32cd32"},
	{Name: "lime-green", Hex: "#32cd32"},
	{Name: "limegreen", Hex: "#32cd32"},
	{Name: "linen", Hex: "#faf0e6"},
	{Name: "medium aquamarine", Hex: "#66cdaa"},
	{Name: "medium-aquamarine", Hex: "#66cdaa"},
	{Name: "mediumaquamarine", Hex: "#66cdaa"},
	{Name: "medium blue", Hex: "#0000cd"},
	{Name: "medium-blue", Hex: "#0000cd"},
	{Name: "mediumblue", Hex: "#0000cd"},
	{Name: "medium orchid", Hex: "#ba55d3"},
	{Name: "medium-orchid", Hex: "#ba55d3"},
	{Name: "mediumorchid", Hex: "#ba55d3"},
	{Name: "medium purple", Hex: "#9370db"},
	{Name: "medium-purple", Hex: "#9370db"},
	{Name: "mediumpurple", Hex: "#9370db"},
	{Name: "medium sea green", Hex: "#3cb371"},
	{Name: "medium-sea-green", Hex: "#3cb371"},
	{Name: "mediumseagreen", Hex: "#3cb371"},
	{Name: "medium slate blue", Hex: "#7b68ee"},
	{Name: "medium-slate-blue", Hex: "#7b68ee"},
	{Name: "mediumslateblue", Hex: "#7b68ee"},
	{Name: "medium spring green", Hex: "#00fa9a"},
	{Name: "medium-spring-green", Hex: "#00fa9a"},
	{Name: "mediumspringgreen", Hex: "#00fa9a"},
	{Name: "medium turquoise", Hex: "#48d1cc"},
	{Name: "medium-turquoise", Hex: "#48d1cc"},
	{Name: "mediumturquoise", Hex: "#48d1cc"},
	{Name: "medium violet red", Hex: "#c71585"},
	{Name: "medium-violet-red", Hex: "#c71585"},
	{Name: "mediumvioletred", Hex: "#c71585"},
	{Name: "midnight blue", Hex: "#191970"},
	{Name: "midnight-blue", Hex: "#191970"},
	{Name: "midnightblue", Hex: "#191970"},
	{Name: "mint cream", Hex: "#f5fffa"},
	{Name: "mint-cream", Hex: "#f5fffa"},
	{Name: "mintcream", Hex: "#f5fffa"},
	{Name: "misty rose", Hex: "#ffe4e1"},
	{Name: "misty-rose", Hex: "#ffe4e1"},
	{Name: "mistyrose", Hex: "#ffe4e1"},
	{Name: "moccasin", Hex: "#ffe4b5"},
	{Name: "navajo white", Hex: "#ffdead"},
	{Name: "navajo-white", Hex: "#ffdead"},
	{Name: "navajowhite", Hex: "#ffdead"},
	{Name: "old lace", Hex: "#fdf5e6"},
	{Name: "old-lace", Hex: "#fdf5e6"},
	{Name: "oldlace", Hex: "#fdf5e6"},
	{Name: "olive drab", Hex: "#6b8e23"},
	{Name: "olive-drab", Hex: "#6b8e23"},
	{Name: "olivedrab", Hex: "#6b8e23"},
	{Name: "orange", Hex: "#ffa500"},
	{Name: "orange red", Hex: "#ff4500"},
	{Name: "orange-red", Hex: "#ff4500"},
	{Name: "orangered", Hex: "#ff4500"},
	{Name: "orchid", Hex: "#da70d6"},
	{Name: "pale goldenrod", Hex: "#eee8aa"},
	{Name: "pale-goldenrod", Hex: "#eee8aa"},
	{Name: "palegoldenrod", Hex: "#eee8aa"},
	{Name: "pale green", Hex: "#98fb98"},
	{Name: "pale-green", Hex: "#98fb98"},
	{Name: "palegreen", Hex: "#98fb98"},
	{Name: "pale turquoise", Hex: "#afeeee"},
	{Name: "pale-turquoise", Hex: "#afeeee"},
	{Name: "paleturquoise", Hex: "#afeeee"},
	{Name: "pale violet red", Hex: "#db7093"},
	{Name: "pale-violet-red", Hex: "#db7093"},
	{Name: "palevioletred", Hex: "#db7093"},
	{Name: "papaya whip", Hex: "#ffefd5"},
	{Name: "papaya-whip", Hex: "#ffefd5"},
	{Name: "papayawhip", Hex: "#ffefd5"},
	{Name: "peach puff", Hex: "#ffdab9"},
	{Name: "peach-puff", Hex: "#ffdab9"},
	{Name: "peachpuff", Hex: "#ffdab9"},
	{Name: "peru", Hex: "#cd853f"},
	{Name: "pink", Hex: "#ffc0cb"},
	{Name: "plum", Hex: "#dda0dd"},
	{Name: "powder blue", Hex: "#b0e0e6"},
	{Name: "powder-blue", Hex: "#b0e0e6"},
	{Name: "powderblue", Hex: "#b0e0e6"},
	{Name: "rebecca purple", Hex: "#663399"},
	{Name: "rebecca-purple", Hex: "#663399"},
	{Name: "rebeccapurple", Hex: "#663399"},
	{Name: "rosy brown", Hex: "#bc8f8f"},
	{Name: "rosy-brown", Hex: "#bc8f8f"},
	{Name: "rosybrown", Hex: "#bc8f8f"},
	{Name: "royal blue", Hex: "#4169e1"},
	{Name: "royal-blue", Hex: "#4169e1"},
	{Name: "royalblue", Hex: "#4169e1"},
	{Name: "saddle brown", Hex: "#8b4513"},
	{Name: "saddle-brown", Hex: "#8b4513"},
	{Name: "saddlebrown", Hex: "#8b4513"},
	{Name: "salmon", Hex: "#fa8072"},
	{Name: "sandy brown", Hex: "#f4a460"},
	{Name: "sandy-brown", Hex: "#f4a460"},
	{Name: "sandysbrown", Hex: "#f4a460"},
	{Name: "sea green", Hex: "#2e8b57"},
	{Name: "sea-green", Hex: "#2e8b57"},
	{Name: "seagreen", Hex: "#2e8b57"},
	{Name: "seashell", Hex: "#fff5ee"},
	{Name: "sienna", Hex: "#a0522d"},
	{Name: "sky blue", Hex: "#87ceeb"},
	{Name: "sky-blue", Hex: "#87ceeb"},
	{Name: "skyblue", Hex: "#87ceeb"},
	{Name: "slate blue", Hex: "#6a5acd"},
	{Name: "slate-blue", Hex: "#6a5acd"},
	{Name: "slateblue", Hex: "#6a5acd"},
	{Name: "slate gray", Hex: "#708090"},
	{Name: "slate-gray", Hex: "#708090"},
	{Name: "slate-grey", Hex: "#708090"},
	{Name: "slategray", Hex: "#708090"},
	{Name: "slategrey", Hex: "#708090"},
	{Name: "snow", Hex: "#fffafa"},
	{Name: "spring green", Hex: "#00ff7f"},
	{Name: "spring-green", Hex: "#00ff7f"},
	{Name: "springgreen", Hex: "#00ff7f"},
	{Name: "steel blue", Hex: "#4682b4"},
	{Name: "steel-blue", Hex: "#4682b4"},
	{Name: "steelblue", Hex: "#4682b4"},
	{Name: "tan", Hex: "#d2b48c"},
	{Name: "thistle", Hex: "#d8bfd8"},
	{Name: "tomato", Hex: "#ff6347"},
	{Name: "turquoise", Hex: "#40e0d0"},
	{Name: "violet", Hex: "#ee82ee"},
	{Name: "wheat", Hex: "#f5deb3"},
	{Name: "white smoke", Hex: "#f5f5f5"},
	{Name: "white-smoke", Hex: "#f5f5f5"},
	{Name: "whitesmoke", Hex: "#f5f5f5"},
	{Name: "yellow green", Hex: "#9acd32"},
	{Name: "yellow-green", Hex: "#9acd32"},
	{Name: "yellowgreen", Hex: "#9acd32"},
}

// Key handles surface-specific keys: navigation, editing, and section toggle.
// In browse mode (no active edit), j/k and ↓/↑ move the cursor, h/l and ←/→
// move between sections, g/G jump to top/bottom, and Enter enters edit mode
// for the selected key. In edit mode, Enter confirms, Esc cancels, Backspace
// deletes, and printable characters append to the input buffer. For enum keys
// the j/k keys navigate the dropdown; for bool keys, space toggles.
//
// Phase 2c: dialogs, color picker, model picker, and help overlay are also
// handled in this method. Phase 3b: the external-file-change dialog is also
// dispatched here. The hint row keybindings (j/k, ↑/↓, ↵, s, q, ?, r) are
// dispatched here.
func (m *ConfigModel) Key(msg tea.KeyPressMsg) tea.Cmd {
	// Dialog overlay takes priority over everything else.
	if m.Data.Dialog != nil {
		if m.Data.Dialog.Kind == dialogExternalFile {
			// External-file dialog is handled separately (Phase 3b) because it
			// needs to know about the external-change state.
			m.handleExternalChangeDialogKey(msg)
			return nil
		}
		return m.handleDialogKey(msg)
	}

	// Model picker overlay.
	if m.Data.ModelPicker != nil {
		return m.handleModelPickerKey(msg)
	}

	// Color picker overlay.
	if m.Data.ColorPicker != nil {
		return m.handleColorPickerKey(msg)
	}

	// If we're in edit mode, handle edit-specific keys.
	if m.Data.Edit != nil {
		return m.handleEditKey(msg)
	}

	// Phase 3b: 'r' reloads when an external change is pending.
	if m.Data.ExternalChange != nil && msg.String() == "r" {
		m.reloadExternalChange()
		return nil
	}

	// Browse mode navigation.
	switch msg.String() {
	case "down", "j":
		m.cursorDown()
	case "up", "k":
		m.cursorUp()
	case "right", "l":
		m.sectionNext()
	case "left", "h":
		m.sectionPrev()
	case "pgdown", "ctrl+d":
		m.pageDown()
	case "pgup", "ctrl+u":
		m.pageUp()
	case "g":
		m.jumpTop()
	case "G":
		m.jumpBottom()
	case "enter":
		return m.enterEdit()
	case "s":
		return m.handleSave()
	case "q":
		return m.handleQuit()
	case "?":
		return m.handleHelp()
	}
	return nil
}

// handleSave triggers a save. Auto-save is always ON, so this is a no-op for
// live-reload keys; it exists for restart-required keys where the user wants to
// see the restart confirmation dialog explicitly.
func (m *ConfigModel) handleSave() tea.Cmd {
	if m.Data.Edit != nil {
		// If editing, confirm the edit first.
		return m.confirmEdit()
	}
	// Check for any restart-required changes that haven't been confirmed yet.
	if len(m.Data.RestartKeys) > 0 {
		m.Data.Dialog = &dialogState{
			Kind:    dialogRestart,
			Title:   "Restart required",
			Message: fmt.Sprintf("The following changes require a restart: %s\nRestart now?", strings.Join(m.Data.RestartKeys, ", ")),
			Options: []string{"Restart now", "Restart later", "Discard changes"},
			Selected: 0,
		}
		return nil
	}
	return nil
}

// handleQuit prompts for quit confirmation if there are unsaved changes.
func (m *ConfigModel) handleQuit() tea.Cmd {
	// Phase 3b: if there's a pending external change, ask about discarding it.
	if m.Data.ExternalChange != nil && m.Data.Dialog == nil {
		m.Data.Dialog = &dialogState{
			Kind:    dialogConfirm,
			Title:   "External change pending",
			Message: "An external change is pending. Discard and quit?",
			Options: []string{"Quit", "Cancel"},
			Selected: 0,
		}
		return nil
	}
	// If we have unsaved restart-required changes, confirm.
	if len(m.Data.RestartKeys) > 0 {
		m.Data.Dialog = &dialogState{
			Kind:    dialogRestart,
			Title:   "Unsaved changes",
			Message: fmt.Sprintf("Changes require restart: %s. Quit without restarting?", strings.Join(m.Data.RestartKeys, ", ")),
			Options: []string{"Quit", "Cancel"},
			Selected: 0,
		}
		return nil
	}
	// If we have an active edit session (not yet confirmed), confirm.
	if m.Data.Edit != nil {
		m.Data.Dialog = &dialogState{
			Kind:    dialogConfirm,
			Title:   "Unsaved changes",
			Message: "Unsaved changes. Quit without saving?",
			Options: []string{"Quit", "Cancel"},
			Selected: 0,
		}
		return nil
	}
	// If we have any unsaved changes at all.
	if m.Data.SaveStatus == SaveStateUnsaved || m.Data.UnsavedChanges {
		m.Data.Dialog = &dialogState{
			Kind:    dialogConfirm,
			Title:   "Unsaved changes",
			Message: "Unsaved changes. Quit without saving?",
			Options: []string{"Quit", "Cancel"},
			Selected: 0,
		}
		return nil
	}
	return nil
}

// handleHelp shows the help overlay documenting keybindings.
func (m *ConfigModel) handleHelp() tea.Cmd {
	if m.Data.Dialog != nil && m.Data.Dialog.Kind == dialogHelp {
		m.Data.Dialog = nil
		return nil
	}
	m.Data.Dialog = &dialogState{
		Kind:    dialogHelp,
		Title:   "Help — config surface",
		Message: "",
		Options: []string{},
	}
	return nil
}

// handleDialogKey processes keys while a modal dialog is active.
func (m *ConfigModel) handleDialogKey(msg tea.KeyPressMsg) tea.Cmd {
	dlg := m.Data.Dialog

	switch msg.String() {
	case "escape", "esc":
		m.Data.Dialog = nil
		return nil
	case "enter":
		return m.confirmDialog()
	case "down", "j":
		if dlg.Selected < len(dlg.Options)-1 {
			dlg.Selected++
		}
		return nil
	case "up", "k":
		if dlg.Selected > 0 {
			dlg.Selected--
		}
		return nil
	}
	return nil
}

// confirmDialog processes the currently selected dialog option.
func (m *ConfigModel) confirmDialog() tea.Cmd {
	dlg := m.Data.Dialog
	if dlg == nil || len(dlg.Options) == 0 {
		m.Data.Dialog = nil
		return nil
	}

	switch dlg.Kind {
	case dialogHelp:
		m.Data.Dialog = nil
		return nil

	case dialogConfirm:
		// "Quit" option confirmed — exit.
		if dlg.Options[dlg.Selected] == "Quit" {
			return m.teaExit()
		}
		m.Data.Dialog = nil
		return nil

	case dialogRestart:
		switch dlg.Options[dlg.Selected] {
		case "Restart now":
			return m.executeRestart()
		case "Restart later":
			m.Data.SaveMsg = "restart deferred — changes saved"
			m.Data.SaveStatus = SaveStateSaved
		case "Discard changes":
			m.Data.RestartKeys = nil
			m.Data.UnsavedChanges = false
			m.Data.SaveStatus = SaveStateLoaded
			m.Data.SaveMsg = "changes discarded"
		}
		m.Data.Dialog = nil
		return nil

	case dialogError:
		m.Data.Dialog = nil
		return nil

	default:
		m.Data.Dialog = nil
		return nil
	}
}

// executeRestart triggers a restart through the transport client.
func (m *ConfigModel) executeRestart() tea.Cmd {
	if m.client == nil {
		m.Data.SaveMsg = "restart not available (no transport)"
		m.Data.SaveStatus = SaveStateError
		return nil
	}

	// First, POST /config to persist the restart-required changes.
	cfg := m.serializeTree()
	return func() tea.Msg {
		result, err := m.client.PostConfig(cfg)
		if err != nil {
			m.Data.SaveStatus = SaveStateError
			m.Data.SaveMsg = "save before restart failed: " + err.Error()
			return nil
		}
		m.Data.RestartKeys = result.RestartRequired

		// Now trigger the restart.
		err = m.client.ExecuteRestart()
		if err != nil {
			m.Data.SaveStatus = SaveStateError
			m.Data.SaveMsg = "restart failed: " + err.Error()
			return nil
		}
		m.Data.SaveStatus = SaveStateSaved
		m.Data.SaveMsg = "restarting…"
		m.Data.Dialog = nil
		return nil
	}
}

// teaExit is a sentinel Cmd that signals the Bubble Tea runtime to quit.
func (m *ConfigModel) teaExit() tea.Cmd {
	return tea.Quit
}

// handleModelPickerKey processes keys while the model picker overlay is active.
func (m *ConfigModel) handleModelPickerKey(msg tea.KeyPressMsg) tea.Cmd {
	pk := m.Data.ModelPicker

	switch msg.String() {
	case "escape", "esc":
		m.Data.ModelPicker = nil
		return nil
	case "enter":
		// Confirm the selected model or custom value.
		if len(pk.Options) > 0 && pk.Selected < len(pk.Options) {
			pk.Custom = pk.Options[pk.Selected]
		} else if pk.Custom != "" {
			// Already has a custom value.
		}
		// If the provider is reachable, test the host before accepting.
		if m.client != nil {
			return m.testHostForModel()
		}
		// No client — accept the value directly (tests).
		m.applyModelPickerSelection()
		return nil
	case "down", "j":
		if pk.Selected < len(pk.Options)-1 {
			pk.Selected++
		}
		return nil
	case "up", "k":
		if pk.Selected > 0 {
			pk.Selected--
		}
		return nil
	}

	// For printable chars, accumulate custom model name.
	if len(msg.Text) == 1 {
		pk.Custom += msg.Text
		pk.Selected = -1 // no longer using the list
		return nil
	}
	return nil
}

// testHostForModel sends POST /test/host for the currently selected model.
func (m *ConfigModel) testHostForModel() tea.Cmd {
	pk := m.Data.ModelPicker
	provider := pk.Provider

	return func() tea.Msg {
		reachable, err := m.client.TestHost(provider, pk.Custom)
		if err != nil || !reachable {
			if err != nil {
				pk.Error = err.Error()
			} else {
				pk.Error = "host unreachable"
			}
			return nil
		}
		m.applyModelPickerSelection()
		return nil
	}
}

// applyModelPickerSelection applies the selected model to the key's value.
func (m *ConfigModel) applyModelPickerSelection() {
	pk := m.Data.ModelPicker
	if pk == nil {
		return
	}
	sec := m.activeSection()
	if sec == nil || pk.keyIndex >= len(sec.keys) {
		m.Data.ModelPicker = nil
		return
	}
	val := pk.Custom
	if val == "" && len(pk.Options) > 0 && pk.Selected >= 0 && pk.Selected < len(pk.Options) {
		val = pk.Options[pk.Selected]
	}
	sec.keys[pk.keyIndex].value = val
	m.Data.ModelPicker = nil
	m.Data.SaveStatus = SaveStateSaving
	if m.client != nil {
		m.Data.SaveMsg = "testing model…"
		return
	}
	m.Data.SaveStatus = SaveStateSaved
	m.Data.SaveMsg = "saved (model verified)"
}

// handleColorPickerKey processes keys while the color picker overlay is active.
func (m *ConfigModel) handleColorPickerKey(msg tea.KeyPressMsg) tea.Cmd {
	pk := m.Data.ColorPicker

	switch msg.String() {
	case "escape", "esc":
		m.Data.ColorPicker = nil
		return nil
	case "enter":
		// Confirm the color and validate it.
		input := m.colorPickerInput(pk)
		if err := validateColor(input); err != nil {
			pk.error = err.Error()
			return nil
		}
		pk.error = ""
		// Apply the color to the key.
		sec := m.activeSection()
		if sec == nil || pk.keyIndex >= len(sec.keys) {
			m.Data.ColorPicker = nil
			return nil
		}
		sec.keys[pk.keyIndex].value = input
		m.Data.ColorPicker = nil
		m.Data.SaveStatus = SaveStateSaving
		if m.client != nil {
			m.Data.SaveMsg = "saving color…"
			return nil
		}
		m.Data.SaveStatus = SaveStateSaved
		m.Data.SaveMsg = "saved (color)"
		return nil
	case "down", "j":
		// Navigate palette.
		if pk.mode == "hex" {
			// In hex mode, move cursor in the hex input.
			if pk.hexInput != "" && len(pk.hexInput) > 0 {
				// Increment the last hex digit.
			}
		} else if pk.mode == "ansi" {
			// Parse current ANSI value, increment.
			if n, err := strconv.Atoi(pk.nameInput); err == nil {
				if n < 255 {
					pk.nameInput = strconv.Itoa(n + 1)
				}
			}
		} else {
			// In name mode, navigate the palette.
			// Find current in palette, move to next.
			current := pk.nameInput
			for i, sw := range pk.palette {
				if strings.ToLower(sw.Name) == strings.ToLower(current) {
					if i+1 < len(pk.palette) {
						pk.nameInput = pk.palette[i+1].Name
					}
					break
				}
			}
		}
		return nil
	case "up", "k":
		if pk.mode == "ansi" {
			if n, err := strconv.Atoi(pk.nameInput); err == nil {
				if n > 0 {
					pk.nameInput = strconv.Itoa(n - 1)
				}
			}
		} else if pk.mode == "name" {
			current := pk.nameInput
			for i, sw := range pk.palette {
				if strings.ToLower(sw.Name) == strings.ToLower(current) {
					if i > 0 {
						pk.nameInput = pk.palette[i-1].Name
					}
					break
				}
			}
		}
		return nil
	}

	// Tab switches between hex/name/ansi modes.
	if msg.String() == "tab" {
		switch pk.mode {
		case "hex":
			pk.mode = "name"
		case "name":
			pk.mode = "ansi"
		case "ansi":
			pk.mode = "hex"
		}
		return nil
	}

	// Printable chars go into the active input.
	if len(msg.Text) == 1 {
		ch := msg.Text[0]
		if ch >= 32 && ch < 127 {
			switch pk.mode {
			case "hex":
				if len(pk.hexInput) < 6 {
					// Only allow hex digits.
					if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
						pk.hexInput += string(ch)
					}
				}
			case "name":
				pk.nameInput += string(ch)
			case "ansi":
				if ch >= '0' && ch <= '9' && len(pk.nameInput) < 3 {
					pk.nameInput += string(ch)
				}
			}
		}
		return nil
	}

	// Backspace.
	if msg.String() == "backspace" {
		switch pk.mode {
		case "hex":
			if len(pk.hexInput) > 0 {
				pk.hexInput = pk.hexInput[:len(pk.hexInput)-1]
			}
		case "name":
			if len(pk.nameInput) > 0 {
				pk.nameInput = pk.nameInput[:len(pk.nameInput)-1]
			}
		case "ansi":
			if len(pk.nameInput) > 0 {
				pk.nameInput = pk.nameInput[:len(pk.nameInput)-1]
			}
		}
		return nil
	}

	return nil
}

// colorPickerInput returns the current value of the color picker, combining
// hex/name/ansi modes.
func (m *ConfigModel) colorPickerInput(pk *colorPickerState) string {
	switch pk.mode {
	case "hex":
		return "#" + pk.hexInput
	case "name":
		return pk.nameInput
	case "ansi":
		return pk.nameInput
	}
	return pk.nameInput
}

// handleEditKey processes keys while an edit session is active.
func (m *ConfigModel) handleEditKey(msg tea.KeyPressMsg) tea.Cmd {
	edit := m.Data.Edit
	sec := m.activeSection()
	if sec == nil || edit.keyIndex >= len(sec.keys) {
		m.exitEdit()
		return nil
	}
	k := sec.keys[edit.keyIndex]

	switch msg.String() {
	case "enter":
		// Confirm the edit.
		return m.confirmEdit()
	case "escape", "esc":
		m.exitEdit()
		return nil
	case "backspace":
		if edit.cursor > 0 {
			edit.input = edit.input[:edit.cursor-1] + edit.input[edit.cursor:]
			edit.cursor--
			edit.error = ""
		}
		return nil
	}

	// For printable characters, append to input.
	if len(msg.Text) == 1 {
		ch := msg.Text[0]
		// Allow backspace, tab, and printable chars.
		if ch >= 32 || ch == 9 {
			edit.input = edit.input[:edit.cursor] + string(ch) + edit.input[edit.cursor:]
			edit.cursor++
			edit.error = ""
		}
		return nil
	}

	// For enum keys, j/k navigate the dropdown.
	if k.kind == "enum" {
		switch msg.String() {
		case "down", "j":
			// Move to next enum value.
			for i, v := range k.enumerable {
				if v == edit.input {
					if i+1 < len(k.enumerable) {
						edit.input = k.enumerable[i+1]
						edit.cursor = len(edit.input)
					}
					break
				}
			}
			return nil
		case "up", "k":
			// Move to previous enum value.
			for i, v := range k.enumerable {
				if v == edit.input {
					if i > 0 {
						edit.input = k.enumerable[i-1]
						edit.cursor = len(edit.input)
					}
					break
				}
			}
			return nil
		}
	}

	// For bool keys, space toggles.
	if k.kind == "bool" {
		if msg.String() == " " || msg.String() == "space" {
			if edit.input == "true" {
				edit.input = "false"
			} else {
				edit.input = "true"
			}
			edit.cursor = len(edit.input)
			return nil
		}
	}

	// Arrow keys move cursor within input.
	switch msg.String() {
	case "left":
		if edit.cursor > 0 {
			edit.cursor--
		}
		return nil
	case "right":
		if edit.cursor < len(edit.input) {
			edit.cursor++
		}
		return nil
	case "home":
		edit.cursor = 0
		return nil
	case "end":
		edit.cursor = len(edit.input)
		return nil
	}

	return nil
}

// enterEdit starts an edit session for the currently selected key.
func (m *ConfigModel) enterEdit() tea.Cmd {
	sec := m.activeSection()
	if sec == nil || m.Data.Cursor >= len(sec.keys) {
		return nil
	}
	k := sec.keys[m.Data.Cursor]

	// Read-only keys cannot be edited.
	if k.readOnly {
		m.Data.SaveStatus = SaveStateError
		m.Data.SaveMsg = "read-only key"
		return nil
	}

	// Phase 2c: host fields open the host test overlay.
	if k.kind == "host" {
		m.enterHostEdit(k)
		return nil
	}

	// Phase 2c: model fields open the model picker overlay.
	if k.kind == "model" {
		m.enterModelPicker(k)
		return nil
	}

	// Phase 2c: color fields open the color picker overlay.
	if k.kind == "color" {
		m.enterColorPicker(k)
		return nil
	}

	// For enum keys, initialize input to the current value.
	input := k.value
	if k.kind == "enum" && len(k.enumerable) > 0 {
		// Find the current value in the enum list, or use the current value.
		found := false
		for _, v := range k.enumerable {
			if v == k.value {
				input = v
				found = true
				break
			}
		}
		if !found {
			input = k.value
		}
	}

	m.Data.Edit = &editState{
		keyIndex: m.Data.Cursor,
		input:    input,
		cursor:   len(input),
	}
	m.Data.SaveStatus = SaveStateUnsaved
	return nil
}

// enterHostEdit starts host editing with live probing (AF-004).
func (m *ConfigModel) enterHostEdit(k keyDef) {
	m.Data.Edit = &editState{
		keyIndex: m.Data.Cursor,
		input:    k.value,
		cursor:   len(k.value),
	}
	m.Data.SaveStatus = SaveStateUnsaved
}

// enterModelPicker starts model editing with provider API lookup (AF-005).
func (m *ConfigModel) enterModelPicker(k keyDef) tea.Cmd {
	// Extract provider name from section (e.g. "agentx.ollama" → "ollama").
	provider := ""
	section := m.activeSection()
	if section != nil {
		parts := strings.Split(section.name, ".")
		if len(parts) > 1 {
			provider = parts[len(parts)-1]
		}
	}
	m.Data.ModelPicker = &modelPickerState{
		keyIndex: m.Data.Cursor,
		Provider: provider,
		Section:  section.name,
		Loading:  true,
		Options:  nil,
	}
	m.Data.SaveStatus = SaveStateSaving
	m.Data.SaveMsg = "loading models…"

	if m.client != nil {
		// Fetch models from the provider API.
		return m.fetchModelList()
	}
	// No client (tests): mark as unreachable.
	m.Data.ModelPicker.Loading = false
	m.Data.ModelPicker.Error = "no transport client"
	return nil
}

// fetchModelList fetches available models from the provider API.
func (m *ConfigModel) fetchModelList() tea.Cmd {
	pk := m.Data.ModelPicker
	return func() tea.Msg {
		models, err := m.client.GetProviderModels(pk.Provider)
		if err != nil {
			pk.Loading = false
			pk.Error = err.Error()
			return nil
		}
		pk.Loading = false
		pk.Options = models
		if len(models) == 0 {
			pk.Error = "no models available"
		}
		m.Data.SaveStatus = SaveStateUnsaved
		m.Data.SaveMsg = ""
		return nil
	}
}

// enterColorPicker starts color editing with a visual picker.
func (m *ConfigModel) enterColorPicker(k keyDef) {
	m.Data.ColorPicker = &colorPickerState{
		keyIndex:  m.Data.Cursor,
		hexInput:  "",
		nameInput: k.value,
		palette:   colorPalette,
		mode:      "name",
	}
	m.Data.SaveStatus = SaveStateUnsaved
}

// confirmEdit validates and applies the current edit.
func (m *ConfigModel) confirmEdit() tea.Cmd {
	if m.Data.Edit == nil {
		return nil
	}
	edit := m.Data.Edit
	sec := m.activeSection()
	if sec == nil || edit.keyIndex >= len(sec.keys) {
		m.exitEdit()
		return nil
	}
	k := sec.keys[edit.keyIndex]

	// Validate the input.
	if err := validateValue(k.kind, edit.input, k); err != nil {
		edit.error = err.Error()
		return nil
	}

	// Phase 2c: host fields are tested against the live endpoint before acceptance (AF-004).
	if k.kind == "host" && m.client != nil {
		provider := m.determineProviderForHost(k)
		return m.testHostAndSave(provider)
	}

	// If validation passes, update the value in the tree.
	sec.keys[edit.keyIndex].value = edit.input
	m.Data.Edit = nil
	m.Data.UnsavedChanges = false
	m.Data.SaveStatus = SaveStateSaving

	// If we have a transport client, POST the change.
	if m.client != nil {
		return m.saveToServer()
	}

	// No client (tests): mark as saved immediately.
	m.Data.SaveStatus = SaveStateSaved
	m.Data.SaveMsg = "auto-saved"
	return nil
}

// determineProviderForHost returns the provider name for a host field based on
// the section name (e.g. "agentx.ollama" → "ollama").
func (m *ConfigModel) determineProviderForHost(k keyDef) string {
	sec := m.activeSection()
	if sec == nil {
		return "ollama"
	}
	parts := strings.Split(sec.name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "ollama"
}

// testHostAndSave tests the host endpoint before accepting the change (AF-004).
func (m *ConfigModel) testHostAndSave(provider string) tea.Cmd {
	edit := m.Data.Edit
	input := edit.input

	return func() tea.Msg {
		reachable, err := m.client.TestHost(provider, input)
		if err != nil || !reachable {
			if err != nil {
				edit.error = "host unreachable: " + err.Error()
			} else {
				edit.error = "host unreachable"
			}
			m.Data.SaveStatus = SaveStateError
			m.Data.SaveMsg = edit.error
			return nil
		}
		// Host is reachable — update the value and save.
		sec := m.activeSection()
		if sec != nil && edit.keyIndex < len(sec.keys) {
			sec.keys[edit.keyIndex].value = input
		}
		m.Data.Edit = nil
		m.Data.SaveStatus = SaveStateSaving
		m.Data.SaveMsg = "host verified, saving…"
		return nil
	}
}

// saveToServer POSTs the current config state to the orchestrator.
//
// Phase 3c: after a successful save, processes any queued external change
// event (PendingExternalEvent) so it is handled once the save completes.
func (m *ConfigModel) saveToServer() tea.Cmd {
	if m.client == nil {
		m.Data.SaveStatus = SaveStateError
		m.Data.SaveMsg = "no transport client"
		return nil
	}

	// Serialize the current tree state to a config map.
	cfg := m.serializeTree()

	// Return a Cmd that does the POST and updates the model with the result.
	return func() tea.Msg {
		result, err := m.client.PostConfig(cfg)
		if err != nil {
			m.Data.SaveStatus = SaveStateError
			m.Data.SaveMsg = "save failed: " + err.Error()
			// Process any queued external event even on save failure,
			// so we don't lose the event while the user is still editing.
			m.ProcessPendingExternalEvent()
			return nil
		}
		if result.Status == "error" {
			m.Data.SaveStatus = SaveStateError
			if len(result.Errors) > 0 {
				m.Data.SaveMsg = strings.Join(result.Errors, "; ")
			} else {
				m.Data.SaveMsg = "save rejected by server"
			}
			// Still process queued event even on server rejection.
			m.ProcessPendingExternalEvent()
			return nil
		}
		m.Data.SaveStatus = SaveStateSaved
		m.Data.SaveMsg = fmt.Sprintf("saved (%d live, %d restart)", len(result.LiveApplied), len(result.RestartRequired))

		// Phase 3c: after a successful save, process any queued external
		// change event that arrived while we were saving.
		m.ProcessPendingExternalEvent()
		return nil
	}
}

// ProcessPendingExternalEvent processes a queued external change event if one
// is pending. Called after a POST /config completes (success or failure) to
// ensure the event is not lost during the write window.
//
// Exported for use by test step definitions.
func (m *ConfigModel) ProcessPendingExternalEvent() {
	if m.Data.PendingExternalEvent == nil {
		return
	}
	ev := m.Data.PendingExternalEvent
	m.Data.PendingExternalEvent = nil

	// Re-run the external change handler. Since the save is now complete
	// (SaveStatus is no longer saving), the handler will proceed normally.
	m.handleExternalConfigChange(*ev)
}

// serializeTree converts the current tree state back to a config map.
func (m *ConfigModel) serializeTree() map[string]any {
	cfg := make(map[string]any)
	for _, sec := range m.Data.Sections {
		secCfg, ok := cfg[sec.name]
		if !ok {
			secCfg = make(map[string]any)
			cfg[sec.name] = secCfg
		}
		secMap := secCfg.(map[string]any)
		for _, k := range sec.keys {
			// Parse the value back to its native type.
			val := parseValue(k.kind, k.value)
			secMap[k.name] = val
		}
	}
	return cfg
}

// exitEdit cancels the current edit session without saving.
func (m *ConfigModel) exitEdit() {
	// Track that there are unsaved changes in the tree (the value was modified
	// in the key list even though the edit was cancelled).
	if m.Data.Edit != nil {
		m.Data.UnsavedChanges = true
	}
	m.Data.Edit = nil
	m.Data.SaveStatus = SaveStateUnsaved
}

// validateValue checks that input is valid for the given kind and key.
func validateValue(kind string, input string, k keyDef) error {
	switch kind {
	case "int":
		n, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if n < k.minValue {
			return fmt.Errorf("must be ≥ %d", k.minValue)
		}
		if k.maxValue > 0 && n > k.maxValue {
			return fmt.Errorf("must be ≤ %d", k.maxValue)
		}
	case "bool":
		if input != "true" && input != "false" {
			return fmt.Errorf("must be true or false")
		}
	case "enum":
		if len(k.enumerable) > 0 {
			found := false
			for _, v := range k.enumerable {
				if v == input {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("must be one of: %s", strings.Join(k.enumerable, ", "))
			}
		}
	case "string", "host", "model":
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("cannot be empty")
		}
	case "color":
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("cannot be empty")
		}
		if err := validateColor(input); err != nil {
			return err
		}
	}
	return nil
}

// validateColor checks that s is a valid color: a named palette entry (case-
// insensitive, with or without hyphens), an ANSI 256 index (0-255), or a hex
// color (#RRGGBB).
func validateColor(s string) error {
	trimmed := strings.TrimSpace(s)
	lower := strings.ToLower(trimmed)

	// Hex: #RRGGBB.
	if strings.HasPrefix(lower, "#") {
		hex := strings.TrimPrefix(lower, "#")
		if len(hex) != 6 {
			return fmt.Errorf("invalid hex color: must be #RRGGBB")
		}
		for _, c := range hex {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return fmt.Errorf("invalid hex color: must be #RRGGBB")
			}
		}
		return nil
	}

	// ANSI 256 index: a decimal integer 0-255.
	if n, err := strconv.Atoi(trimmed); err == nil && n >= 0 && n <= 255 {
		return nil
	}

	// Named color: look up in the palette (case-insensitive, hyphens normalized).
	for _, sw := range colorPalette {
		if strings.ToLower(sw.Name) == lower {
			return nil
		}
	}

	return fmt.Errorf("unknown color: %q (try a name like 'cyan', hex like '#00afaf', or ANSI index 0-255)", s)
}

// parseValue converts a string value back to its native type.
func parseValue(kind string, s string) any {
	switch kind {
	case "bool":
		return s == "true"
	case "int":
		n, err := strconv.Atoi(s)
		if err == nil {
			return n
		}
		return s
	default:
		return s
	}
}

// FetchConfig fetches the current effective config and schema from the
// orchestrator, merging them into the model's internal tree. Returns nil on
// success, or an error if either read fails.
//
// Deprecated: use FetchConfig (no args) instead. Kept for backward compatibility.
func (m *ConfigModel) FetchConfigOld(cl *transporthttp.Client) error {
	m.client = cl
	return m.FetchConfig()
}

// handleExternalConfigChange is triggered by a config_changed event. It
// re-fetches the full config from the orchestrator, diffs it against the
// current in-memory state, highlights changed keys, and shows a reload prompt.
//
// Phase 3b: AF-008 — external file change detection.
// Phase 3c: adds conflict resolution (TUI wins when unsaved), surface-side
// debounce for rapid successive changes, and event queueing while a POST /config
// is in flight.
func (m *ConfigModel) handleExternalConfigChange(ev state.Event) {
	// Phase 3c: if a save is in flight, queue the event so it is processed
	// after the save completes. This prevents a lost event during the critical
	// write window.
	if m.Data.SaveStatus == SaveStateSaving {
		m.Data.PendingExternalEvent = &ev
		m.Data.SaveMsg = "save in progress; external change queued"
		return
	}

	// Phase 3c: surface-side debounce for rapid successive changes.
	// If another event arrived within the debounce window, update the timestamp
	// on any existing ExternalChange but do not re-trigger the full handler.
	if m.Data.LastExternalEventAt > 0 {
		elapsed := ev.Epoch - m.Data.LastExternalEventAt
		if elapsed < surfaceDebounceWindowSec {
			// Coalesce: update the existing ExternalChange's timestamp if we have one.
			if m.Data.ExternalChange != nil {
				m.Data.ExternalChange.ChangedAt = ev.Epoch
			}
			m.Data.LastExternalEventAt = ev.Epoch
			return
		}
	}
	m.Data.LastExternalEventAt = ev.Epoch

	// Pull the payload path if available.
	var changePath string
	if payload, ok := ev.Payload.(map[string]any); ok {
		if p, ok := payload["path"].(string); ok {
			changePath = p
		}
	}

	// Phase 3c: conflict resolution — if the TUI has unsaved changes, prefer
	// TUI state over the external file change.
	if m.Data.SaveStatus == SaveStateUnsaved || m.Data.UnsavedChanges {
		m.Data.ExternalChangeDiscarded = true
		m.Data.ExternalChange = &externalChangeState{
			Path:      changePath,
			ChangedAt: ev.Epoch,
		}
		m.Data.Dialog = &dialogState{
			Kind:     dialogExternalFile,
			Title:    "TUI changes take precedence",
			Message:  "You have unsaved changes in the TUI. The external file change is discarded.",
			Options:  []string{"Keep TUI changes", "Discard"},
			Selected: 0, // "Keep TUI changes" is the default — TUI wins.
			Source:   changePath,
		}
		m.Data.SaveStatus = SaveStateUnsaved
		m.Data.SaveMsg = fmt.Sprintf("external change discarded (TUI changes take precedence)")
		return
	}

	// Snapshot the current sections for diffing before we overwrite them.
	oldSections := snapshotSections(m.Data.Sections)

	// Re-fetch from the orchestrator.
	if err := m.FetchConfig(); err != nil {
		m.Data.SaveStatus = SaveStateError
		m.Data.SaveMsg = "external change: fetch failed: " + err.Error()
		return
	}

	// Compute hashes for quick-equal check.
	oldHash := configHash(oldSections)
	newHash := configHash(m.Data.Sections)

	if oldHash == newHash {
		m.Data.ExternalChange = &externalChangeState{
			Path:      changePath,
			ChangedAt: ev.Epoch,
			OldHash:   oldHash,
			NewHash:   newHash,
		}
		m.Data.SaveStatus = SaveStateSaved
		m.Data.SaveMsg = "config refreshed (no changes detected)"
		return
	}

	// Compute the actual diff against the snapshot.
	changedKeys := diffSections(oldSections, m.Data.Sections)

	m.Data.ExternalChange = &externalChangeState{
		Path:        changePath,
		ChangedAt:   ev.Epoch,
		OldHash:     oldHash,
		NewHash:     newHash,
		ChangedKeys: changedKeys,
	}

	// Build the highlight set.
	m.Data.HighlightedKeys = make(map[string]bool, len(changedKeys))
	for _, k := range changedKeys {
		m.Data.HighlightedKeys[k] = true
	}

	// Show the reload prompt dialog.
	m.Data.Dialog = &dialogState{
		Kind:    dialogExternalFile,
		Title:   "File changed externally",
		Message: fmt.Sprintf("agentx.toml was modified externally. Reload?"),
		Options: []string{"Reload", "Keep changes", "Discard changes"},
		Selected: 0,
		Source:   changePath,
	}
	m.Data.SaveStatus = SaveStateSaved
	m.Data.SaveMsg = fmt.Sprintf("external change detected (%d keys)", len(changedKeys))
}

// snapshotSections deep-copies a slice of sections so the original data is
// preserved while the live Data.Sections may be overwritten by FetchConfig.
func snapshotSections(sections []section) []section {
	snap := make([]section, len(sections))
	for i, sec := range sections {
		secCopy := sec
		keysCopy := make([]keyDef, len(sec.keys))
		copy(keysCopy, sec.keys)
		secCopy.keys = keysCopy
		snap[i] = secCopy
	}
	return snap
}

// configHash computes a stable hash of the current section tree for change
// detection. It serializes sections into a canonical string form and returns it
// as a sha256 hex digest.
//
// ConfigHashForTest exposes this for use in Godog integration tests.
func ConfigHashForTest(cfg map[string]any) string {
	sections := BuildTree(cfg, map[string]provider.SchemaField{}).Sections
	return configHash(sections)
}

func configHash(sections []section) string {
	var b strings.Builder
	for _, sec := range sections {
		b.WriteString(sec.name)
		b.WriteByte(':')
		for _, k := range sec.keys {
			b.WriteString(k.name)
			b.WriteByte('=')
			b.WriteString(k.value)
			b.WriteByte(',')
		}
	}
	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h[:8]) // truncated for quick comparison
}

// diffSections compares old and new section trees and returns the list of
// dotted "section.key" paths whose values differ. Sections and keys are
// matched by name within each section; new or removed keys are reported as
// additions/deletions.
func diffSections(oldSections, newSections []section) []string {
	// Index old keys by section+key name.
	oldByKey := make(map[string]string) // "section.key" → old value
	for _, sec := range oldSections {
		for _, k := range sec.keys {
			oldByKey[sec.name+"."+k.name] = k.value
		}
	}

	// Index new keys.
	newByKey := make(map[string]string) // "section.key" → new value
	for _, sec := range newSections {
		for _, k := range sec.keys {
			newByKey[sec.name+"."+k.name] = k.value
		}
	}

	// Collect changed keys in deterministic order.
	var changed []string
	seen := make(map[string]bool)

	// Check old keys against new.
	for path, oldVal := range oldByKey {
		newVal, exists := newByKey[path]
		if !exists {
			changed = append(changed, path+" (removed)")
			seen[path] = true
		} else if oldVal != newVal {
			changed = append(changed, path)
			seen[path] = true
		} else {
			// Key exists unchanged in both — mark as seen so it's not
			// double-reported in the "new keys" pass.
			seen[path] = true
		}
	}
	// Check for new keys not in old.
	for path := range newByKey {
		if !seen[path] {
			changed = append(changed, path+" (added)")
		}
	}
	return changed
}

// handleExternalChangeDialog processes keys while the external-file dialog is active.
func (m *ConfigModel) handleExternalChangeDialogKey(msg tea.KeyPressMsg) {
	dlg := m.Data.Dialog
	if dlg == nil || dlg.Kind != dialogExternalFile {
		return
	}
	switch msg.String() {
	case "escape", "esc":
		m.dismissExternalChange()
	case "enter":
		m.confirmExternalDialog()
	case "down", "j":
		if dlg.Selected < len(dlg.Options)-1 {
			dlg.Selected++
		}
	case "up", "k":
		if dlg.Selected > 0 {
			dlg.Selected--
		}
	}
}

// confirmExternalDialog processes the selected option in the external-file dialog.
//
// Phase 3c: handles both the standard "File changed externally" dialog
// (Reload / Keep changes / Discard changes) and the conflict-resolution dialog
// ("TUI changes take precedence" / Keep TUI changes / Discard) that appears
// when the user has unsaved TUI changes.
func (m *ConfigModel) confirmExternalDialog() {
	dlg := m.Data.Dialog
	if dlg == nil || dlg.Kind != dialogExternalFile {
		m.Data.Dialog = nil
		return
	}
	switch dlg.Options[dlg.Selected] {
	case "Reload":
		m.reloadExternalChange()
	case "Keep changes":
		m.dismissExternalChange()
		m.Data.SaveMsg = "TUI changes take precedence (external change discarded)"
	case "Keep TUI changes":
		m.dismissExternalChange()
		m.Data.ExternalChangeDiscarded = false
		m.Data.SaveMsg = "TUI changes take precedence (external change discarded)"
	case "Discard":
		m.dismissExternalChange()
		m.Data.ExternalChangeDiscarded = false
		m.Data.SaveStatus = SaveStateLoaded
		m.Data.SaveMsg = "external change discarded"
	case "Discard changes":
		m.reloadExternalChange()
		m.Data.SaveMsg = "changes discarded, reloaded from file"
	}
	m.Data.Dialog = nil
}

// reloadExternalChange re-fetches the config and clears the highlight.
func (m *ConfigModel) reloadExternalChange() {
	if m.client == nil {
		m.Data.SaveMsg = "reload failed: no transport"
		m.Data.SaveStatus = SaveStateError
		return
	}
	if err := m.FetchConfig(); err != nil {
		m.Data.SaveStatus = SaveStateError
		m.Data.SaveMsg = "reload failed: " + err.Error()
		return
	}
	m.Data.ExternalChange = nil
	m.Data.HighlightedKeys = nil
	m.Data.SaveStatus = SaveStateSaved
	m.Data.SaveMsg = "config reloaded from file"
}

// dismissExternalChange clears the external-change state without re-fetching.
func (m *ConfigModel) dismissExternalChange() {
	m.Data.ExternalChange = nil
	m.Data.Dialog = nil
	m.Data.SaveMsg = "external change dismissed"
}

// cursorUp moves the cursor up by one, staying within the active section.
func (m *ConfigModel) cursorUp() {
	if m.Data.Cursor > 0 {
		m.Data.Cursor--
	}
}

// cursorDown moves the cursor down by one, staying within the active section.
func (m *ConfigModel) cursorDown() {
	if m.Data.Cursor < m.activeKeyCount()-1 {
		m.Data.Cursor++
	}
}

// pageUp scrolls up by one page.
func (m *ConfigModel) pageUp() {
	step := m.visibleRowCount()
	m.Data.Cursor -= step
	if m.Data.Cursor < 0 {
		m.Data.Cursor = 0
	}
}

// pageDown scrolls down by one page.
func (m *ConfigModel) pageDown() {
	step := m.visibleRowCount()
	m.Data.Cursor += step
	if m.Data.Cursor >= m.activeKeyCount() {
		m.Data.Cursor = m.activeKeyCount() - 1
	}
}

// sectionNext moves to the next section.
func (m *ConfigModel) sectionNext() {
	if m.Data.Selected < len(m.Data.Sections)-1 {
		m.Data.Selected++
		m.Data.Cursor = 0
	}
}

// sectionPrev moves to the previous section.
func (m *ConfigModel) sectionPrev() {
	if m.Data.Selected > 0 {
		m.Data.Selected--
		m.Data.Cursor = 0
	}
}

// jumpTop moves the cursor to the first key.
func (m *ConfigModel) jumpTop() {
	m.Data.Cursor = 0
}

// jumpBottom moves the cursor to the last key.
func (m *ConfigModel) jumpBottom() {
	n := m.activeKeyCount()
	if n > 0 {
		m.Data.Cursor = n - 1
	}
}

// activeSection returns the currently selected section, or nil if out of range.
func (m *ConfigModel) activeSection() *section {
	if m.Data.Selected < 0 || m.Data.Selected >= len(m.Data.Sections) {
		return nil
	}
	return &m.Data.Sections[m.Data.Selected]
}

// activeKeyCount returns the number of keys in the active section.
func (m *ConfigModel) activeKeyCount() int {
	s := m.activeSection()
	if s == nil {
		return 0
	}
	return len(s.keys)
}

// visibleRowCount returns the number of visible rows (reserving title + hint).
func (m *ConfigModel) visibleRowCount() int {
	n := m.Height - 2
	if n <= 0 {
		return 1
	}
	return n
}

// Apply folds one session event into the surface's projection. The config
// surface fetches its data via the transport (GET /config) at launch time,
// so live events are ignored in Phase 2a/2b. Two-way sync (Phase 3) uses
// config_changed events here to reload the tree and highlight diffs.
func (m *ConfigModel) Apply(ev state.Event) {
	if ev.ContentType == state.ContentConfigChanged {
		m.handleExternalConfigChange(ev)
	}
}

// SetSize sets the render area.
func (m *ConfigModel) SetSize(width, height int) {
	m.Width = width
	m.Height = max(0, height)
}

// CapturesKeys reports whether the surface is capturing free-form text input.
// True during edit mode, false otherwise (SS-8).
func (m *ConfigModel) CapturesKeys() bool {
	return m.Data.Edit != nil
}
