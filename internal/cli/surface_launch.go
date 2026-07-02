package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"agentx/internal/config"
	"agentx/internal/session"
	"agentx/internal/surfaces"
	"agentx/internal/surfaces/client"
	contextsurface "agentx/internal/surfaces/context"
	"agentx/internal/surfaces/contextviz"
	"agentx/internal/surfaces/workmemory"
	transporthttp "agentx/internal/transport/http"
)

// LaunchArgs are the resolved inputs to a `surface launch` (canonical or alias).
type LaunchArgs struct {
	SurfaceKind string
	Session     string // selector: session name or id (optional; auto-resolved if empty)
	Connect     string // orchestrator endpoint; empty auto-resolves from disk (SS-5)
	Token       string // ephemeral attach token; empty auto-resolves from disk (SS-5)
	SessionRoot string // session storage root for auto-resolution; empty uses config default
}

// LaunchResult is the outcome of a successful attach, for operator output.
type LaunchResult struct {
	SurfaceID   string
	SurfaceKind string
	SessionID   string
	SessionName string
	Endpoint    string
	Token       string // resolved attach token, for the surface client to reuse
}

// Launch validates the launch arguments, attaches the surface to the running
// orchestrator at args.Connect, and returns the registration outcome. Failures
// carry a deterministic reason category (validation | auth | transport | conflict)
// via *transporthttp.AttachError. See docs/implementation/02 (Normative CLI
// Specification, Launch Implementation TRN-5).
func Launch(ctx context.Context, args LaunchArgs) (LaunchResult, error) {
	if !surfaces.KnownKind(args.SurfaceKind) {
		return LaunchResult{}, validation("unknown surface kind %q; run a known surface (e.g. files, config, context)", args.SurfaceKind)
	}

	// Resolve the endpoint + token: explicit flags win; otherwise discover the active
	// session from disk (SS-5) so a same-machine peer needs no copied token.
	endpoint, token, err := resolveConnection(ctx, args)
	if err != nil {
		return LaunchResult{}, err
	}
	if err := checkLocalSafe(endpoint); err != nil {
		return LaunchResult{}, err
	}

	cl := transporthttp.NewClient(endpoint)

	// Resolve the session: the endpoint must be reachable and, when a selector is
	// given, its active session must match it (rule 2: exactly one active session).
	id, err := cl.CurrentSession(ctx)
	if err != nil {
		return LaunchResult{}, err
	}
	if sel := strings.TrimSpace(args.Session); sel != "" && sel != id.ID && sel != id.Name {
		return LaunchResult{}, validation("session %q does not match the running session (%s / %s)", sel, id.Name, id.ID)
	}

	reg, err := cl.Register(ctx, args.SurfaceKind, endpoint, token, nil)
	if err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{
		SurfaceID:   reg.SurfaceID,
		SurfaceKind: reg.SurfaceKind,
		SessionID:   reg.SessionID,
		SessionName: reg.SessionName,
		Endpoint:    endpoint,
		Token:       token,
	}, nil
}

// resolveConnection returns the endpoint + attach token to attach with. Explicit
// --connect uses the provided values; otherwise it discovers the active session's
// published endpoint and 0600 token from the session root on disk and returns the
// newest reachable one (SS-5).
func resolveConnection(ctx context.Context, args LaunchArgs) (endpoint, token string, err error) {
	if strings.TrimSpace(args.Connect) != "" {
		return args.Connect, args.Token, nil
	}

	root := args.SessionRoot
	if root == "" {
		paths, perr := config.DefaultPaths()
		if perr != nil {
			return "", "", validation("cannot resolve session root: %v", perr)
		}
		root = paths.SessionRoot()
	}

	cands, derr := session.NewStore(root).DiscoverTransports()
	if derr != nil {
		return "", "", validation("cannot read published sessions: %v", derr)
	}
	if len(cands) == 0 {
		return "", "", validation("no running session found; start agentx, or pass --connect and --token")
	}

	// Only consider sessions whose server is actually answering.
	var live []session.ActiveTransport
	for _, c := range cands {
		if reachable(ctx, c.Endpoint) {
			live = append(live, c)
		}
	}
	if len(live) == 0 {
		return "", "", &transporthttp.AttachError{
			Category: "transport",
			Message:  "found published session(s) but none are reachable; pass --connect",
		}
	}

	// With a selector, attach to the matching session (by name or id). Without one,
	// a single live session is unambiguous; multiple require disambiguation so a
	// surface never silently attaches to the wrong session.
	if sel := strings.TrimSpace(args.Session); sel != "" {
		for _, c := range live {
			if sel == c.SessionName || sel == c.SessionID {
				return c.Endpoint, c.Token, nil
			}
		}
		return "", "", validation("no running session matches %q; running: %s", sel, sessionNames(live))
	}
	if len(live) == 1 {
		return live[0].Endpoint, live[0].Token, nil
	}
	return "", "", validation("multiple sessions are running (%s); pass --session <name>", sessionNames(live))
}

// sessionNames lists the discoverable session names (falling back to id) for an
// ambiguity error.
func sessionNames(live []session.ActiveTransport) string {
	names := make([]string, len(live))
	for i, c := range live {
		if c.SessionName != "" {
			names[i] = c.SessionName
		} else {
			names[i] = c.SessionID
		}
	}
	return strings.Join(names, ", ")
}

// reachable probes whether an orchestrator is answering at endpoint.
func reachable(ctx context.Context, endpoint string) bool {
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_, err := transporthttp.NewClient(endpoint).CurrentSession(rctx)
	return err == nil
}

// RunSurface launches a surface: it attaches (Launch), then — if the kind has a
// TUI — runs the surface program until the user quits; otherwise it reports the
// headless attach (the surface registered but has no UI yet). This is the
// `agentx surface launch` entry point used by cmd/agentx, keeping the
// surface-client dependency out of the command package.
func RunSurface(ctx context.Context, args LaunchArgs) error {
	res, err := Launch(ctx, args)
	if err != nil {
		return err
	}
	// Working memory is read-write and document-based, so it runs its own program
	// rather than the event-stream surface host.
	if args.SurfaceKind == "working-memory" {
		return workmemory.Run(ctx, workmemory.Options{
			Endpoint:    res.Endpoint,
			Token:       res.Token,
			SurfaceID:   res.SurfaceID,
			SessionName: res.SessionName,
		})
	}
	// Context visualizer is read-only and document-based (it polls the assembled
	// context composition), so it runs its own program too.
	if args.SurfaceKind == "context-visualizer" {
		return contextviz.Run(ctx, contextviz.Options{
			Endpoint:    res.Endpoint,
			Token:       res.Token,
			SurfaceID:   res.SurfaceID,
			SessionName: res.SessionName,
		})
	}

	if surface, title, ok := surfaceModelFor(args.SurfaceKind, res); ok {
		return client.Run(ctx, client.Options{
			Endpoint:  res.Endpoint,
			Token:     res.Token,
			SurfaceID: res.SurfaceID,
			Title:     title,
			Surface:   surface,
		})
	}
	fmt.Printf("surface attached headless: %s (%s) — no TUI for this kind yet\n", res.SurfaceID, res.SurfaceKind)
	fmt.Printf("session: %s / %s\n", res.SessionName, res.SessionID)
	fmt.Printf("endpoint: %s\n", res.Endpoint)
	return nil
}

// surfaceModelFor returns the SurfaceModel + title for a launchable kind, or
// false when the kind has no TUI yet. Concrete surfaces register here as they land.
func surfaceModelFor(kind string, res LaunchResult) (client.SurfaceModel, string, bool) {
	switch kind {
	case "context":
		return contextsurface.New(), "context · " + res.SessionName, true
	default:
		return nil, "", false
	}
}

// checkLocalSafe enforces a loopback connect endpoint (v1 policy, rule 3).
func checkLocalSafe(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return validation("invalid --connect endpoint %q", endpoint)
	}
	host := u.Hostname()
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return nil
	default:
		return validation("endpoint %q is not a local-safe address (v1 allows loopback only)", endpoint)
	}
}

func validation(format string, a ...any) error {
	return &transporthttp.AttachError{Category: "validation", Message: fmt.Sprintf(format, a...)}
}
