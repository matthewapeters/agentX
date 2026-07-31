package hooks

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Factory builds a hook instance from its config options. The returned value
// must implement SyncHook or AsyncHook, matching Config.Async — Build reports
// an error if it implements neither the declared interface.
type Factory func(options map[string]any) (any, error)

// Available is the compiled-in registry of hook factories by name, resolved
// against config at startup. Registration is config-driven, not hardcoded: a
// future hook implementation registers itself here (by name, typically from
// its own package's init), and only runs when a HooksConfigPath config file
// names it. Empty in this pass — no hook implementations ship yet.
var Available = map[string]Factory{}

// Config is one [[hook]] entry in a hooks config file.
type Config struct {
	Name    string         `toml:"name"`
	Async   bool           `toml:"async"`
	Options map[string]any `toml:"options"`
}

// LoadConfig reads hook registrations from a TOML file:
//
//	[[hook]]
//	name  = "my_indexer"
//	async = true
//	[hook.options]
//	interval_seconds = 30
//
// A missing file (or empty path) yields no configs (not an error) — the same
// convention as tools.LoadBlacklist/LoadApprovals.
func LoadConfig(path string) ([]Config, error) {
	if path == "" {
		return nil, nil
	}
	var doc struct {
		Hook []Config `toml:"hook"`
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load hooks config %s: %w", path, err)
	}
	return doc.Hook, nil
}

// Build resolves configs against Available into a ready Registry. An unknown
// hook name, or one whose constructed value doesn't implement the interface
// its config declares (Async), is an error — a config typo should fail loudly
// at startup, not silently run with fewer hooks than configured. An empty
// configs list (today's default — nothing ships in HooksConfigPath) returns
// an empty, fully functional no-op Registry.
func Build(configs []Config) (*Registry, error) {
	r := NewRegistry()
	for _, c := range configs {
		factory, ok := Available[c.Name]
		if !ok {
			return nil, fmt.Errorf("hook %q: no such hook registered", c.Name)
		}
		instance, err := factory(c.Options)
		if err != nil {
			return nil, fmt.Errorf("hook %q: %w", c.Name, err)
		}
		if c.Async {
			h, ok := instance.(AsyncHook)
			if !ok {
				return nil, fmt.Errorf("hook %q: configured async but does not implement AsyncHook", c.Name)
			}
			r.RegisterAsync(h)
			continue
		}
		h, ok := instance.(SyncHook)
		if !ok {
			return nil, fmt.Errorf("hook %q: configured sync but does not implement SyncHook", c.Name)
		}
		r.RegisterSync(h)
	}
	return r, nil
}
