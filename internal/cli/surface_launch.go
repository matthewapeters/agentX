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
	configsurface "agentx/internal/surfaces/config"
	contextsurface "agentx/internal/surfaces/context"
	"agentx/internal/surfaces/contextviz"
	"agentx/internal/surfaces/logs"
	"agentx/internal/surfaces/workmemory"
	transporthttp "agentx/internal/transport/http"
)

// LaunchArgs are the resolved inputs to a `surface launch` (canonical or alias).
type LaunchArgs struct {
	SurfaceKind    string
	Session        string        // selector: session name or id (optional; auto-resolved if empty)
	Connect        string        // orchestrator endpoint; empty auto-resolves from disk (SS-5)
	Token          string        // ephemeral attach token; empty auto-resolves from disk (SS-5)
	SessionRoot    string        // session storage root for auto-resolution; empty uses config default
	ConnectTimeout time.Duration // auto-discovery retry window; zero uses defaultConnectTimeout; ignored when Connect is set
	SessionInTitle bool          // show the session name in the surface's title (default off; see LaunchTitleSession)
}

// LaunchTitleSession returns the session label to embed in a launched surface's
// title. It is empty unless the operator opted in with --session-in-title: under a
// display harness the surrounding panes already name the session, so titles stay
// uncluttered by default; a standalone launch opts in so peer surfaces can be told
// apart. The chat's "attach surfaces" widget names the session regardless, to guide
// operators who launch surfaces by hand. Because each surface already omits the
// " · <session>" suffix when the name is empty, gating the name here is the single
// point that governs every surface's title.
func LaunchTitleSession(args LaunchArgs, res LaunchResult) string {
	if args.SessionInTitle {
		return res.SessionName
	}
	return ""
}

// defaultConnectTimeout bounds how long an auto-discovery launch waits for a
// just-started server to publish its transport and begin answering before giving
// up. A surface is often launched at the same instant as the `agentx` it attaches
// to (e.g. every pane of a multiplexer layout starts at once), so discovery polls
// rather than losing the race — this is what lets a layout drop the `sleep` hack.
const defaultConnectTimeout = 10 * time.Second

// connectRetryInterval is the poll cadence while waiting for a session to appear.
const connectRetryInterval = 150 * time.Millisecond

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

	// A surface is frequently launched at the same instant as the server it attaches
	// to, so the server may not have published its transport — or may not be answering
	// — on the first look. Poll discovery until a matching live session appears or the
	// timeout elapses, so a concurrent launch attaches instead of losing the race.
	// Terminal outcomes (unreadable root, genuine ambiguity) do not retry.
	timeout := args.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnectTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		ep, tok, retry, rerr := resolveOnce(ctx, args, root)
		if rerr == nil {
			return ep, tok, nil
		}
		if !retry || time.Now().After(deadline) {
			return "", "", rerr
		}
		select {
		case <-ctx.Done():
			return "", "", rerr
		case <-time.After(min(connectRetryInterval, time.Until(deadline))):
		}
	}
}

// resolveOnce performs a single discovery pass: it reads the published sessions,
// keeps the ones whose server is answering, and resolves the selector. The bool
// reports whether a failure is worth retrying — the awaited session may still be
// starting (nothing published yet, or published but not answering) — versus
// terminal (an unreadable root, or genuine ambiguity that waiting cannot resolve).
func resolveOnce(ctx context.Context, args LaunchArgs, root string) (endpoint, token string, retry bool, err error) {
	cands, derr := session.NewStore(root).DiscoverTransports()
	if derr != nil {
		return "", "", false, validation("cannot read published sessions: %v", derr)
	}
	if len(cands) == 0 {
		return "", "", true, validation("no running session found; start agentx, or pass --connect and --token")
	}

	// Only consider sessions whose server is actually answering.
	var live []session.ActiveTransport
	for _, c := range cands {
		if reachable(ctx, c.Endpoint) {
			live = append(live, c)
		}
	}
	if len(live) == 0 {
		return "", "", true, &transporthttp.AttachError{
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
				return c.Endpoint, c.Token, false, nil
			}
		}
		// The named session may still be booting, so keep polling for it.
		return "", "", true, validation("no running session matches %q; running: %s", sel, sessionNames(live))
	}
	if len(live) == 1 {
		return live[0].Endpoint, live[0].Token, false, nil
	}
	return "", "", false, validation("multiple sessions are running (%s); pass --session <name>", sessionNames(live))
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
	// One gate governs every surface's title: surfaces show the session only when a
	// non-empty name reaches them, so passing the toggled value here cascades.
	titleSession := LaunchTitleSession(args, res)
	// Working memory is read-write and document-based, so it runs its own program
	// rather than the event-stream surface host.
	if args.SurfaceKind == "working-memory" {
		return workmemory.Run(ctx, workmemory.Options{
			Endpoint:    res.Endpoint,
			Token:       res.Token,
			SurfaceID:   res.SurfaceID,
			SessionName: titleSession,
		})
	}
	// Context visualizer is read-only and document-based (it polls the assembled
	// context composition), so it runs its own program too.
	if args.SurfaceKind == "context-visualizer" {
		return contextviz.Run(ctx, contextviz.Options{
			Endpoint:    res.Endpoint,
			Token:       res.Token,
			SurfaceID:   res.SurfaceID,
			SessionName: titleSession,
		})
	}

	if surface, title, ok := surfaceModelFor(args.SurfaceKind, res, titleSession); ok {
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
func surfaceModelFor(kind string, res LaunchResult, sessionName string) (client.SurfaceModel, string, bool) {
	switch kind {
	case "config":
		title := "config"
		if sessionName != "" {
			title += " · " + sessionName
		}
		return configsurface.New(transporthttp.NewClient(res.Endpoint), res.Token), title, true
	case "context":
		title := "context"
		if sessionName != "" {
			title += " · " + sessionName
		}
		return contextsurface.New(transporthttp.NewClient(res.Endpoint), res.Token), title, true
	case "logs":
		title := "logs"
		if sessionName != "" {
			title += " · " + sessionName
		}
		return logs.New(), title, true
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
