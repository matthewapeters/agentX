package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"agentx/internal/surfaces"
	"agentx/internal/surfaces/client"
	contextsurface "agentx/internal/surfaces/context"
	transporthttp "agentx/internal/transport/http"
)

// LaunchArgs are the resolved inputs to a `surface launch` (canonical or alias).
type LaunchArgs struct {
	SurfaceKind string
	Session     string // selector: session name or id
	Connect     string // orchestrator endpoint (alias port is mapped to this)
	Token       string // ephemeral attach token
}

// LaunchResult is the outcome of a successful attach, for operator output.
type LaunchResult struct {
	SurfaceID   string
	SurfaceKind string
	SessionID   string
	SessionName string
	Endpoint    string
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
	if strings.TrimSpace(args.Session) == "" {
		return LaunchResult{}, validation("a --session selector is required")
	}
	if err := checkLocalSafe(args.Connect); err != nil {
		return LaunchResult{}, err
	}

	client := transporthttp.NewClient(args.Connect)

	// Resolve the session: the endpoint must be reachable and its active session
	// must match the selector (rule 2: exactly one active session).
	id, err := client.CurrentSession(ctx)
	if err != nil {
		return LaunchResult{}, err
	}
	if args.Session != id.ID && args.Session != id.Name {
		return LaunchResult{}, validation("session %q does not match the running session (%s / %s)", args.Session, id.Name, id.ID)
	}

	reg, err := client.Register(ctx, args.SurfaceKind, args.Connect, args.Token, nil)
	if err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{
		SurfaceID:   reg.SurfaceID,
		SurfaceKind: reg.SurfaceKind,
		SessionID:   reg.SessionID,
		SessionName: reg.SessionName,
		Endpoint:    args.Connect,
	}, nil
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
	if surface, title, ok := surfaceModelFor(args.SurfaceKind, res); ok {
		return client.Run(ctx, client.Options{
			Endpoint:  args.Connect,
			Token:     args.Token,
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
