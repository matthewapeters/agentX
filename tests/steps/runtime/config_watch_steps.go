// Package runtimesteps adds Godog step definitions for the config watch
// domain (Phase 3a: filesystem watcher + config_changed event publishing).
package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/config"
	"agentx/internal/runtime"
	"agentx/internal/state"
	transporthttp "agentx/internal/transport/http"
)

// configWatchWorld carries per-scenario state for config-watch scenarios.
type configWatchWorld struct {
	configDir    string
	configPath   string
	orch         *runtime.Orchestrator
	events       []state.Event
	mu           sync.Mutex
	sub          *state.Subscription
	bus          *state.Bus
	configPathSetting string
}

// InitializeConfigWatch registers the config-watch steps.
func InitializeConfigWatch(sc *godog.ScenarioContext) {
	w := &configWatchWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "agentx-watch-test-*")
		if err != nil {
			return ctx, err
		}
		w.configDir = dir
		w.configPath = filepath.Join(dir, "agentx.toml")
		w.configPathSetting = w.configPath
		w.orch = nil
		w.events = nil
		w.sub = nil
		w.bus = nil
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.orch != nil {
			_ = w.orch.Shutdown(context.Background())
		}
		if w.configDir != "" {
			_ = os.RemoveAll(w.configDir)
		}
		*w = configWatchWorld{}
		return ctx, nil
	})

	// --- Setup ---
	sc.Step(`^an orchestrator with config watcher enabled$`, w.setupOrchestrator)
	sc.Step(`^the config file exists at "([^"]*)"$`, w.ensureConfigFile)
	sc.Step(`^a subscriber is attached to the bus$`, w.attachSubscriber)

	// --- Actions ---
	sc.Step(`^I edit the config file \(simulate external editor\)$`, w.editConfigFile)
	sc.Step(`^I rapidly edit the config file (\d+) times \(within debounce window\)$`, w.rapidEditConfigFile)
	sc.Step(`^I wait for debounce window$`, w.waitDebounce)

	// --- Assertions ---
	sc.Step(`^a "config_changed" event is published on the bus$`, w.configChangedPublished)
	sc.Step(`^the config_changed event has session_id set$`, w.configChangedHasSessionID)
	sc.Step(`^the config_changed event payload contains "([^"]*)"$`, w.payloadContainsKey)
	sc.Step(`^the config_changed event payload path equals "([^"]*)"$`, w.payloadPathEquals)
	sc.Step(`^the config_changed event payload path ends with "([^"]*)"$`, w.payloadPathEndsWith)
	sc.Step(`^at most (\d+) "config_changed" events are published on the bus$`, w.atMostConfigChangedEvents)
	sc.Step(`^the subscriber receives a "([^"]*)" event$`, w.subscriberReceivesEvent)
}

func (w *configWatchWorld) setupOrchestrator() error {
	// Write a minimal valid TOML config so the deployment path exists.
	if err := os.WriteFile(w.configPath, []byte("[agentx.ollama]\nhost = \"localhost:11434\"\n"), 0644); err != nil {
		return err
	}

	sessionRoot := filepath.Join(w.configDir, "sessions")
	if err := os.MkdirAll(sessionRoot, 0755); err != nil {
		return err
	}

	// Override XDG_CONFIG_HOME via env so config.DefaultPaths() uses our temp dir.
	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", w.configDir)
	defer func() {
		if oldXdg == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", oldXdg)
		}
	}()

	o := runtime.New(runtime.Settings{
		SessionRoot:        sessionRoot,
		Provider:           "ollama",
		OllamaHost:         "localhost:11434",
		OllamaModel:        "phi4-mini:3.8b",
		TransportEnabled:   false,
		MaxWidgetLines:     20,
		InputMaxLines:      8,
		ToolTimeoutSeconds: 30,
		ToolOutputMaxBytes: 65536,
		ConfigWatcherPath:  w.configPathSetting,
	},
	)
	w.orch = o
	if err := o.Start(); err != nil {
		return fmt.Errorf("start orchestrator: %w", err)
	}
	w.bus = o.Bus()

	// Always attach a subscriber so we can capture events.
	w.sub = w.bus.Subscribe()
	go func() {
		for ev := range w.sub.C {
			w.mu.Lock()
			w.events = append(w.events, ev)
			w.mu.Unlock()
		}
	}()
	return nil
}

// ensureConfigFile creates a minimal valid config file at the given path.
func (w *configWatchWorld) ensureConfigFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte("[agentx.ollama]\nhost = \"localhost:11434\"\n"), 0644)
	}
	return nil
}

// attachSubscriber creates a bus subscriber that captures all events.
func (w *configWatchWorld) attachSubscriber() error {
	if w.bus == nil {
		return fmt.Errorf("no orchestrator bus available")
	}
	w.sub = w.bus.Subscribe()
	go func() {
		for ev := range w.sub.C {
			w.mu.Lock()
			w.events = append(w.events, ev)
			w.mu.Unlock()
		}
	}()
	return nil
}

// editConfigFile writes to the config file to simulate an external edit.
func (w *configWatchWorld) editConfigFile() error {
	if w.configPath == "" {
		return fmt.Errorf("no config path")
	}
	return os.WriteFile(w.configPath, []byte("[agentx.ollama]\nhost = \"localhost:11435\"\n"), 0644)
}

// waitDebounce sleeps long enough for the debounce window to elapse.
func (w *configWatchWorld) waitDebounce() error {
	time.Sleep(3 * debounceWindow)
	return nil
}

// rapidEditConfigFile writes to the config file N times in rapid succession.
func (w *configWatchWorld) rapidEditConfigFile(nStr string) error {
	n, err := strconv.Atoi(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}
	if w.configPath == "" {
		return fmt.Errorf("no config path")
	}
	for i := 0; i < n; i++ {
		if err := os.WriteFile(w.configPath, []byte(fmt.Sprintf("[agentx.ollama]\nhost = \"localhost:%d\"\n", 11434+i)), 0644); err != nil {
			return err
		}
	}
	// Wait for debouncing to complete.
	time.Sleep(5 * debounceWindow)
	return nil
}

// configChangedPublished checks that a config_changed event was published.
func (w *configWatchWorld) configChangedPublished() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			return nil
		}
	}
	return fmt.Errorf("no config_changed event published (got %d events)", len(w.events))
}

// configChangedHasSessionID checks that the config_changed event has a session_id.
func (w *configWatchWorld) configChangedHasSessionID() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			if ev.SessionID == "" {
				return fmt.Errorf("config_changed event has empty session_id")
			}
			return nil
		}
	}
	return fmt.Errorf("no config_changed event found")
}

// payloadContainsKey checks that the config_changed event payload contains the given key.
func (w *configWatchWorld) payloadContainsKey(key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			if payload, ok := ev.Payload.(map[string]any); ok {
				if _, hasKey := payload[key]; hasKey {
					return nil
				}
			}
			return fmt.Errorf("config_changed event payload does not contain key %q", key)
		}
	}
	return fmt.Errorf("no config_changed event found")
}

// payloadPathEndsWith checks that the config_changed event payload path ends with the expected suffix.
func (w *configWatchWorld) payloadPathEndsWith(expected string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			if payload, ok := ev.Payload.(map[string]any); ok {
				if path, hasKey := payload["path"]; hasKey {
					if pathStr, ok := path.(string); ok && strings.HasSuffix(pathStr, expected) {
						return nil
					}
				}
			}
			return fmt.Errorf("config_changed event payload does not have path ending with %q", expected)
		}
	}
	return fmt.Errorf("no config_changed event found")
}

// payloadPathEquals checks that the config_changed event payload path equals the expected value.
func (w *configWatchWorld) payloadPathEquals(expected string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			if payload, ok := ev.Payload.(map[string]any); ok {
				if path, hasKey := payload["path"]; hasKey {
					if pathStr, ok := path.(string); ok && pathStr == expected {
						return nil
					}
				}
			}
			return fmt.Errorf("config_changed event payload does not have path=%q", expected)
		}
	}
	return fmt.Errorf("no config_changed event found")
}

// atMostConfigChangedEvents checks that at most n config_changed events were published.
func (w *configWatchWorld) atMostConfigChangedEvents(max int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	count := 0
	for _, ev := range w.events {
		if ev.ContentType == state.ContentConfigChanged {
			count++
		}
	}
	if count > max {
		return fmt.Errorf("expected at most %d config_changed events, got %d", max, count)
	}
	return nil
}

// subscriberReceivesEvent checks that the bus subscriber received an event of the given type.
func (w *configWatchWorld) subscriberReceivesEvent(contentType string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if string(ev.ContentType) == contentType {
			return nil
		}
	}
	return fmt.Errorf("subscriber did not receive event of type %q (got %d events)", contentType, len(w.events))
}

// debounceWindow mirrors the value in config/watch.go for test consistency.
const debounceWindow = 100 * time.Millisecond

// --- Unused imports kept for future use ---
var _ = config.DefaultPaths
var _ = transporthttp.NewServer
var _ = strings.TrimSpace
