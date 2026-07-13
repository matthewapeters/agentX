package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// OutputOverride is a persisted per-tool decision for how to handle
// output_max_bytes truncation, set via an "always" choice on the
// oversized-output recovery gate (TOOL-6). Decision is "use_truncated" (accept
// the labeled, partial result without asking) or "expand" (re-run with
// CapBytes, ceiling-clamped) — CapBytes is meaningless for "use_truncated".
type OutputOverride struct {
	Tool     string `toml:"tool"`
	Decision string `toml:"decision"`
	CapBytes int    `toml:"cap_bytes,omitempty"`
}

// LoadOutputOverrides reads persisted per-tool output-size overrides from a TOML
// file:
//
//	[[override]]
//	tool = "read_file"
//	decision = "expand"
//	cap_bytes = 262144
//
// A missing file yields no entries (not an error).
func LoadOutputOverrides(path string) ([]OutputOverride, error) {
	if path == "" {
		return nil, nil
	}
	var doc struct {
		Override []OutputOverride `toml:"override"`
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load output overrides %s: %w", path, err)
	}
	return doc.Override, nil
}

// SaveOutputOverrides writes the overrides to path (creating parent dirs),
// replacing any previous content.
func SaveOutputOverrides(path string, entries []OutputOverride) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output overrides dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output overrides %s: %w", path, err)
	}
	defer f.Close()
	doc := struct {
		Override []OutputOverride `toml:"override"`
	}{Override: entries}
	if err := toml.NewEncoder(f).Encode(doc); err != nil {
		return fmt.Errorf("write output overrides %s: %w", path, err)
	}
	return nil
}

// OutputOverrides holds persisted per-tool output-size decisions in memory,
// keyed by tool id, so a later truncation from the same tool can be resolved
// without re-prompting.
type OutputOverrides struct {
	entries map[string]OutputOverride
}

// NewOutputOverrides seeds a holder from loaded entries (e.g. LoadOutputOverrides'
// result).
func NewOutputOverrides(seed []OutputOverride) *OutputOverrides {
	m := make(map[string]OutputOverride, len(seed))
	for _, e := range seed {
		m[e.Tool] = e
	}
	return &OutputOverrides{entries: m}
}

// Get returns the remembered override for tool, if any.
func (o *OutputOverrides) Get(tool string) (OutputOverride, bool) {
	e, ok := o.entries[tool]
	return e, ok
}

// Set records (or replaces) the override for its tool.
func (o *OutputOverrides) Set(e OutputOverride) {
	o.entries[e.Tool] = e
}

// All returns every override, sorted by tool for stable on-disk output.
func (o *OutputOverrides) All() []OutputOverride {
	keys := make([]string, 0, len(o.entries))
	for k := range o.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]OutputOverride, 0, len(keys))
	for _, k := range keys {
		out = append(out, o.entries[k])
	}
	return out
}
