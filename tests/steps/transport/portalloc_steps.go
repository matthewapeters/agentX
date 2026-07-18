package transportsteps

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"

	"agentx/internal/session"
	transporthttp "agentx/internal/transport/http"
)

type portWorld struct {
	host       string
	port       int
	rangeStart int
	rangeEnd   int

	occupied  net.Listener
	allocated net.Listener
	allocErr  error

	store *session.Store
	dir   string
	id    session.Identity
}

// registerPortSteps wires the port-allocation and endpoint-publication steps (TRN-4).
func registerPortSteps(sc *godog.ScenarioContext) {
	w := &portWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.occupied != nil {
			_ = w.occupied.Close()
		}
		if w.allocated != nil {
			_ = w.allocated.Close()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = portWorld{}
		return ctx, nil
	})

	sc.Step(`^a discovered free port$`, w.discoverPort)
	sc.Step(`^that port is occupied$`, w.occupyPort)
	sc.Step(`^the allocator binds a range of size 1 at that port$`, w.allocateSingle)
	sc.Step(`^the allocator binds a range from that port spanning (\d+)$`, w.allocateSpan)
	sc.Step(`^a listener is bound on that port$`, w.boundOnPort)
	sc.Step(`^a listener is bound on a different port in the range$`, w.boundDifferent)
	sc.Step(`^allocation fails with a range-exhausted error$`, w.allocationFailed)

	sc.Step(`^a session store with a created session$`, w.createdSession)
	sc.Step(`^the endpoint "([^"]*)" is published$`, w.publishEndpoint)
	sc.Step(`^reading the session transport metadata returns endpoint "([^"]*)"$`, w.readEndpoint)

	// attach-token persistence + discovery (SS-5).
	sc.Step(`^the attach token "([^"]*)" is published$`, w.publishToken)
	sc.Step(`^reading the session attach token returns "([^"]*)"$`, w.readToken)
	sc.Step(`^the attach token file mode is 0600$`, w.tokenMode)
	sc.Step(`^discovering transports finds endpoint "([^"]*)" with token "([^"]*)"$`, w.discoverFinds)
	sc.Step(`^discovering transports finds nothing$`, w.discoverEmpty)
}

func (w *portWorld) discoverPort() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	w.host = "127.0.0.1"
	w.port = ln.Addr().(*net.TCPAddr).Port
	return ln.Close()
}

func (w *portWorld) occupyPort() error {
	ln, err := net.Listen("tcp", net.JoinHostPort(w.host, fmt.Sprintf("%d", w.port)))
	if err != nil {
		return err
	}
	w.occupied = ln
	return nil
}

func (w *portWorld) allocateSingle() error {
	w.rangeStart, w.rangeEnd = w.port, w.port
	w.allocated, w.allocErr = transporthttp.Allocate(w.host, w.rangeStart, w.rangeEnd)
	return nil
}

func (w *portWorld) allocateSpan(n int) error {
	w.rangeStart, w.rangeEnd = w.port, w.port+n-1
	w.allocated, w.allocErr = transporthttp.Allocate(w.host, w.rangeStart, w.rangeEnd)
	return nil
}

func (w *portWorld) boundOnPort() error {
	if w.allocErr != nil {
		return fmt.Errorf("allocation failed: %v", w.allocErr)
	}
	if got := w.allocated.Addr().(*net.TCPAddr).Port; got != w.port {
		return fmt.Errorf("bound port = %d, want %d", got, w.port)
	}
	return nil
}

func (w *portWorld) boundDifferent() error {
	if w.allocErr != nil {
		return fmt.Errorf("allocation failed: %v", w.allocErr)
	}
	got := w.allocated.Addr().(*net.TCPAddr).Port
	if got == w.port {
		return fmt.Errorf("bound the occupied port %d", got)
	}
	if got < w.rangeStart || got > w.rangeEnd {
		return fmt.Errorf("bound port %d outside range [%d, %d]", got, w.rangeStart, w.rangeEnd)
	}
	return nil
}

func (w *portWorld) allocationFailed() error {
	if w.allocErr == nil {
		return fmt.Errorf("expected a range-exhausted error, got a bound listener")
	}
	return nil
}

func (w *portWorld) createdSession() error {
	dir, err := os.MkdirTemp("", "agentx-transport-*")
	if err != nil {
		return err
	}
	w.dir = dir
	w.store = session.NewStore(dir)
	w.id, err = w.store.Create()
	return err
}

func (w *portWorld) publishEndpoint(endpoint string) error {
	return w.store.WriteTransport(w.id.ID, session.TransportInfo{
		SessionID: w.id.ID,
		Endpoint:  endpoint,
	})
}

func (w *portWorld) readEndpoint(want string) error {
	info, err := w.store.ReadTransport(w.id.ID)
	if err != nil {
		return err
	}
	if info.Endpoint != want {
		return fmt.Errorf("endpoint = %q, want %q", info.Endpoint, want)
	}
	return nil
}

func (w *portWorld) publishToken(token string) error {
	return w.store.WriteAttachToken(w.id.ID, token)
}

func (w *portWorld) readToken(want string) error {
	got, err := w.store.ReadAttachToken(w.id.ID)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("attach token = %q, want %q", got, want)
	}
	return nil
}

func (w *portWorld) tokenMode() error {
	fi, err := os.Stat(filepath.Join(w.store.Dir(w.id.ID), "attach-token"))
	if err != nil {
		return err
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		return fmt.Errorf("attach token file mode = %o, want 600", perm)
	}
	return nil
}

func (w *portWorld) discoverFinds(endpoint, token string) error {
	cands, err := w.store.DiscoverTransports()
	if err != nil {
		return err
	}
	for _, c := range cands {
		if c.Endpoint == endpoint && c.Token == token {
			return nil
		}
	}
	return fmt.Errorf("no discovered transport with endpoint %q token %q (got %d)", endpoint, token, len(cands))
}

func (w *portWorld) discoverEmpty() error {
	cands, err := w.store.DiscoverTransports()
	if err != nil {
		return err
	}
	if len(cands) != 0 {
		return fmt.Errorf("expected no discoverable transports, got %d", len(cands))
	}
	return nil
}
