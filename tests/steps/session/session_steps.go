// Package sessionsteps holds Godog step definitions for the session domain
// (CHT-A2). Steps live in an importable (non-_test) file so suite runners under
// tests/suites can aggregate them.
package sessionsteps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cucumber/godog"

	"agentx/internal/session"
)

// adjNounForm matches a default-style session name — adjective-participial-noun
// (internal/session/names.go's defaultNamer) — allowing an optional numeric
// collision suffix (e.g. "brave-running-otter" or "brave-running-otter-2").
var adjNounForm = regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+(-[0-9]+)?$`)

type sessionWorld struct {
	dir      string
	namer    func() string
	sessions []session.Identity
	genned   string // last generated/minted session name (name-helper steps)
}

// InitializeScenario registers the session-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerIdentitySteps(sc)
	registerPersistenceSteps(sc)
	registerWorkingMemorySteps(sc)
}

// registerIdentitySteps wires the session-identity steps (CHT-A2).
func registerIdentitySteps(sc *godog.ScenarioContext) {
	w := &sessionWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "agentx-sess-")
		if err != nil {
			return ctx, err
		}
		w.dir = dir
		w.namer = nil
		w.sessions = nil
		w.genned = ""
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		return ctx, nil
	})

	sc.Step(`^an empty session root$`, w.emptyRoot)
	sc.Step(`^the name generator always yields "([^"]*)"$`, w.namerYields)
	sc.Step(`^a session is created$`, w.createSession)
	sc.Step(`^another session is created$`, w.createSession)
	sc.Step(`^the session has a non-empty session_id$`, w.hasID)
	sc.Step(`^the session has a non-empty session_name$`, w.hasName)
	sc.Step(`^the session directory exists with an events folder$`, w.dirWithEvents)
	sc.Step(`^session\.json records the same id and name$`, w.metadataMatches)
	sc.Step(`^the first session_name is "([^"]*)"$`, w.firstNameIs)
	sc.Step(`^the second session_name is "([^"]*)"$`, w.secondNameIs)
	sc.Step(`^a default session name is generated$`, w.genName)
	sc.Step(`^a unique session name is minted$`, w.mintUnique)
	sc.Step(`^the resulting name has the adjective-noun form$`, w.formOK)
	sc.Step(`^the minted name differs from the existing session's name$`, w.mintedDiffers)
}

func (w *sessionWorld) genName() error {
	w.genned = session.GenerateName()
	return nil
}

func (w *sessionWorld) mintUnique() error {
	name, err := session.NewStore(w.dir).UniqueName()
	if err != nil {
		return err
	}
	w.genned = name
	return nil
}

func (w *sessionWorld) formOK() error {
	if !adjNounForm.MatchString(w.genned) {
		return fmt.Errorf("name %q is not adjective-noun form", w.genned)
	}
	return nil
}

func (w *sessionWorld) mintedDiffers() error {
	existing, err := w.first()
	if err != nil {
		return err
	}
	if w.genned == existing.Name {
		return fmt.Errorf("minted name %q collides with existing session name", w.genned)
	}
	return nil
}

func (w *sessionWorld) emptyRoot() error {
	// Temp root already created in Before; nothing else required.
	return nil
}

func (w *sessionWorld) namerYields(name string) error {
	w.namer = func() string { return name }
	return nil
}

func (w *sessionWorld) createSession() error {
	store := session.NewStore(w.dir)
	var opts []session.Option
	if w.namer != nil {
		opts = append(opts, session.WithNamer(w.namer))
	}
	id, err := store.Create(opts...)
	if err != nil {
		return err
	}
	w.sessions = append(w.sessions, id)
	return nil
}

func (w *sessionWorld) first() (session.Identity, error) {
	if len(w.sessions) == 0 {
		return session.Identity{}, fmt.Errorf("no session created")
	}
	return w.sessions[0], nil
}

func (w *sessionWorld) hasID() error {
	s, err := w.first()
	if err != nil {
		return err
	}
	if s.ID == "" {
		return fmt.Errorf("session_id is empty")
	}
	return nil
}

func (w *sessionWorld) hasName() error {
	s, err := w.first()
	if err != nil {
		return err
	}
	if s.Name == "" {
		return fmt.Errorf("session_name is empty")
	}
	return nil
}

func (w *sessionWorld) dirWithEvents() error {
	s, err := w.first()
	if err != nil {
		return err
	}
	events := filepath.Join(w.dir, s.ID, "events")
	info, err := os.Stat(events)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("expected events dir at %s", events)
	}
	return nil
}

func (w *sessionWorld) metadataMatches() error {
	s, err := w.first()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(w.dir, s.ID, "session.json"))
	if err != nil {
		return fmt.Errorf("read session.json: %w", err)
	}
	var got session.Identity
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("decode session.json: %w", err)
	}
	if got.ID != s.ID || got.Name != s.Name {
		return fmt.Errorf("session.json {id=%q name=%q} != created {id=%q name=%q}",
			got.ID, got.Name, s.ID, s.Name)
	}
	return nil
}

func (w *sessionWorld) nameAt(idx int, want string) error {
	if len(w.sessions) <= idx {
		return fmt.Errorf("session %d not created", idx+1)
	}
	if w.sessions[idx].Name != want {
		return fmt.Errorf("session %d name = %q, want %q", idx+1, w.sessions[idx].Name, want)
	}
	return nil
}

func (w *sessionWorld) firstNameIs(want string) error  { return w.nameAt(0, want) }
func (w *sessionWorld) secondNameIs(want string) error { return w.nameAt(1, want) }
