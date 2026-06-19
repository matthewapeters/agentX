package main

// Applet is the minimal interface every concrete applet must satisfy.
// Render is the single pure function: given terminal dimensions it returns
// the exact lines that should appear on screen.  It must be deterministic
// for a fixed struct state so tests can seed dimensions and assert output.
//
// All runtime concerns (cursor, ANSI escape sequences to clear/redraw,
// resize polling, HTTP API) are handled by AppletBase, not here.
type Applet interface {
	// Name returns a stable applet identifier used in /render responses.
	Name() string

	// Render produces the screen lines for the given terminal dimensions.
	// The returned slice must not be modified by the caller; implementations
	// should return a fresh slice each time.
	Render(height, width int) []string
}
