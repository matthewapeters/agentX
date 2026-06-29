package surfacesteps

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/surfaces"
)

type registryWorld struct {
	token    surfaces.AttachToken
	registry *surfaces.Registry
	last     surfaces.Registration
	lastErr  error
}

// registerRegistrySteps wires the surface-registry / attach-token steps (TRN-1).
func registerRegistrySteps(sc *godog.ScenarioContext) {
	w := &registryWorld{}

	sc.Step(`^a session registry with a minted attach token$`, w.mintRegistry)
	sc.Step(`^a "([^"]*)" surface registers with the valid token$`, w.registerKind)
	sc.Step(`^a "([^"]*)" surface "([^"]*)" is registered with the valid token$`, w.registerKindID)
	sc.Step(`^a "([^"]*)" surface "([^"]*)" registers with the valid token$`, w.registerKindID)
	sc.Step(`^a surface "([^"]*)" registers with the valid token$`, w.registerID)
	sc.Step(`^a "([^"]*)" surface registers with the token "([^"]*)"$`, w.registerKindToken)
	sc.Step(`^surface "([^"]*)" is shut down$`, w.shutdown)
	sc.Step(`^the registration is accepted$`, w.accepted)
	sc.Step(`^the registration is rejected with category "([^"]*)"$`, w.rejected)
	sc.Step(`^the surface lifecycle state is "([^"]*)"$`, w.lastLifecycle)
	sc.Step(`^the surface "([^"]*)" lifecycle state is "([^"]*)"$`, w.lifecycleOf)
	sc.Step(`^the stored record carries the token fingerprint not the raw token$`, w.fingerprintNotRaw)
	sc.Step(`^the record has the required surface-registration fields$`, w.requiredFields)
	sc.Step(`^the registry has (\d+) surfaces?$`, w.surfaceCount)
	sc.Step(`^the registry lists surfaces in order "([^"]*)"$`, w.listOrder)
}

func (w *registryWorld) mintRegistry() error {
	tok, err := surfaces.MintToken()
	if err != nil {
		return err
	}
	w.token = tok
	w.registry = surfaces.NewRegistry(tok, "sess-1", "calm-otter")
	w.last = surfaces.Registration{}
	w.lastErr = nil
	return nil
}

// register runs a registration and stashes the result/error for assertions.
func (w *registryWorld) register(req surfaces.RegisterRequest) error {
	w.last, w.lastErr = w.registry.Register(req)
	return nil
}

func (w *registryWorld) registerKind(kind string) error {
	return w.register(surfaces.RegisterRequest{
		SurfaceKind:      kind,
		TransportAddress: "http://127.0.0.1:7777",
		Token:            w.token.Raw(),
	})
}

func (w *registryWorld) registerKindID(kind, id string) error {
	return w.register(surfaces.RegisterRequest{
		SurfaceID:        id,
		SurfaceKind:      kind,
		TransportAddress: "http://127.0.0.1:7777",
		Token:            w.token.Raw(),
	})
}

func (w *registryWorld) registerID(id string) error {
	return w.register(surfaces.RegisterRequest{
		SurfaceID:        id,
		SurfaceKind:      "surface",
		TransportAddress: "http://127.0.0.1:7777",
		Token:            w.token.Raw(),
	})
}

func (w *registryWorld) registerKindToken(kind, token string) error {
	return w.register(surfaces.RegisterRequest{
		SurfaceKind:      kind,
		TransportAddress: "http://127.0.0.1:7777",
		Token:            token,
	})
}

func (w *registryWorld) shutdown(id string) error {
	return w.registry.Shutdown(id)
}

func (w *registryWorld) accepted() error {
	if w.lastErr != nil {
		return fmt.Errorf("registration rejected: %v", w.lastErr)
	}
	return nil
}

func (w *registryWorld) rejected(category string) error {
	if w.lastErr == nil {
		return fmt.Errorf("registration was accepted, expected rejection with category %q", category)
	}
	var re *surfaces.RegisterError
	if !errors.As(w.lastErr, &re) {
		return fmt.Errorf("error %v is not a *RegisterError", w.lastErr)
	}
	if re.Category != category {
		return fmt.Errorf("category = %q, want %q", re.Category, category)
	}
	return nil
}

func (w *registryWorld) lastLifecycle(state string) error {
	if w.last.LifecycleState != state {
		return fmt.Errorf("lifecycle = %q, want %q", w.last.LifecycleState, state)
	}
	return nil
}

func (w *registryWorld) lifecycleOf(id, state string) error {
	reg, ok := w.registry.Get(id)
	if !ok {
		return fmt.Errorf("surface %q not found", id)
	}
	if reg.LifecycleState != state {
		return fmt.Errorf("surface %q lifecycle = %q, want %q", id, reg.LifecycleState, state)
	}
	return nil
}

func (w *registryWorld) fingerprintNotRaw() error {
	if w.last.AttachTokenFingerprint != w.token.Fingerprint() {
		return fmt.Errorf("fingerprint = %q, want %q", w.last.AttachTokenFingerprint, w.token.Fingerprint())
	}
	if w.last.AttachTokenFingerprint == w.token.Raw() {
		return fmt.Errorf("record stored the raw token as its fingerprint")
	}
	return nil
}

func (w *registryWorld) requiredFields() error {
	r := w.last
	missing := []string{}
	if r.SurfaceID == "" {
		missing = append(missing, "surface_id")
	}
	if r.SurfaceKind == "" {
		missing = append(missing, "surface_kind")
	}
	if r.TransportAddress == "" {
		missing = append(missing, "transport_address")
	}
	if r.Capabilities == nil {
		missing = append(missing, "capabilities")
	}
	if r.LifecycleState == "" {
		missing = append(missing, "lifecycle_state")
	}
	if r.SessionID == "" {
		missing = append(missing, "session_id")
	}
	if r.SessionName == "" {
		missing = append(missing, "session_name")
	}
	if r.AttachTokenFingerprint == "" {
		missing = append(missing, "attach_token_fingerprint")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (w *registryWorld) surfaceCount(n int) error {
	if got := len(w.registry.List()); got != n {
		return fmt.Errorf("registry has %d surfaces, want %d", got, n)
	}
	return nil
}

func (w *registryWorld) listOrder(want string) error {
	ids := []string{}
	for _, r := range w.registry.List() {
		ids = append(ids, r.SurfaceID)
	}
	got := strings.Join(ids, ", ")
	if got != want {
		return fmt.Errorf("surface order = %q, want %q", got, want)
	}
	return nil
}
