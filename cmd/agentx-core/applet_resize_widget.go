package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// AppletRenderSnapshot — canonical render payload for every applet.
// ---------------------------------------------------------------------------

// AppletRenderSnapshot is the standardised, self-describing render payload
// exposed by every applet's GET /render endpoint.  Dimensions are always
// included so callers can assert correctness without separately tracking size.
type AppletRenderSnapshot struct {
	AppletName string   `json:"applet_name"`
	Sequence   int      `json:"sequence"`
	Height     int      `json:"height"`
	Width      int      `json:"width"`
	Lines      []string `json:"lines"`
	Frame      string   `json:"frame"`
	UpdatedAt  string   `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// AppletBase — embed in every concrete applet struct.
// Owns: thread-safe snapshot storage, HTTP API server, resize poll loop.
// ---------------------------------------------------------------------------

// AppletBase should be embedded in every concrete applet struct.  It provides:
//   - Thread-safe storage of the latest AppletRenderSnapshot.
//   - A self-contained HTTP server exposing GET /health and GET /render.
//   - A resize polling loop that calls Applet.Render on dimension changes and
//     redraws the terminal via drawAppletFrame.
type AppletBase struct {
	mu       sync.RWMutex
	snapshot AppletRenderSnapshot
}

// StoreSnapshot records a new render snapshot thread-safely, increments
// Sequence, joins lines into Frame, and stamps UpdatedAt.
func (b *AppletBase) StoreSnapshot(name string, height, width int, lines []string) AppletRenderSnapshot {
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}
	frame := strings.Join(lines, "\n")
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snapshot.AppletName = name
	b.snapshot.Sequence++
	b.snapshot.Height = height
	b.snapshot.Width = width
	b.snapshot.Lines = append([]string(nil), lines...)
	b.snapshot.Frame = frame
	b.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return b.snapshot
}

// Snapshot returns a safe copy of the latest render snapshot.
func (b *AppletBase) Snapshot() AppletRenderSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := b.snapshot
	cp.Lines = append([]string(nil), b.snapshot.Lines...)
	return cp
}

// StartAPIServer binds an HTTP server at addr, exposing GET /health and
// GET /render.  addr may be "host:port" or empty/"host:0" for a random port.
// Returns the bound address.  The server shuts down when ctx is cancelled.
func (b *AppletBase) StartAPIServer(ctx context.Context, addr string, errOut io.Writer) (string, error) {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", strings.TrimSpace(addr))
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/render", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(b.Snapshot())
	})
	server := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			if errOut != nil {
				_, _ = fmt.Fprintf(errOut, "[applet:%s] API server error: %v\n", b.Snapshot().AppletName, serveErr)
			}
		}
	}()
	return listener.Addr().String(), nil
}

// RunResizeLoop polls terminal dimensions and redraws via applet.Render on
// changes.  Runs until ctx is cancelled.
func (b *AppletBase) RunResizeLoop(ctx context.Context, applet Applet, out io.Writer, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 120 * time.Millisecond
	}
	hideTerminalCursor(out)
	defer showTerminalCursor(out)

	snap := b.Snapshot()
	if err := drawAppletFrame(out, snap.Frame); err != nil {
		return err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			height, width := resolveWidgetPaneSizeForWriter(out)
			current := b.Snapshot()
			if height == current.Height && width == current.Width {
				continue
			}
			lines := applet.Render(height, width)
			updated := b.StoreSnapshot(applet.Name(), height, width, lines)
			if err := drawAppletFrame(out, updated.Frame); err != nil {
				return err
			}
		}
	}
}

// drawAppletFrame clears the terminal and writes the full frame string.
func drawAppletFrame(out io.Writer, frame string) error {
	_, err := fmt.Fprintf(out, "\033[H\033[2J%s", frame)
	return err
}

// ---------------------------------------------------------------------------
// ResizeBorderApplet — simplest concrete Applet; draws a box border.
// ---------------------------------------------------------------------------

// ResizeBorderApplet is the canonical minimal applet.  It draws a rectangular
// border that exactly fills the available terminal dimensions.  Used as a
// zellij resize-sensitivity test harness and as a reference implementation
// of the Applet interface.
type ResizeBorderApplet struct {
	AppletBase
}

func (a *ResizeBorderApplet) Name() string { return "resize-border" }

// Render produces border lines for (height x width).
// Pure function: identical inputs always produce identical output.
func (a *ResizeBorderApplet) Render(height, width int) []string {
	return buildResizeBorderLines(height, width)
}

func buildResizeBorderLines(height int, width int) []string {
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}
	if width == 1 {
		if height == 1 {
			return []string{"+"}
		}
		lines := make([]string, 0, height)
		lines = append(lines, "+")
		for i := 0; i < height-2; i++ {
			lines = append(lines, "|")
		}
		lines = append(lines, "+")
		return lines
	}
	horizontal := strings.Repeat("-", width-2)
	top := "+" + horizontal + "+"
	if height == 1 {
		return []string{top}
	}
	inner := "|" + strings.Repeat(" ", width-2) + "|"
	lines := make([]string, 0, height)
	lines = append(lines, top)
	for i := 0; i < height-2; i++ {
		lines = append(lines, inner)
	}
	lines = append(lines, top)
	return lines
}

// ---------------------------------------------------------------------------
// runAppletResizeCommand — entry point for --applet-resize CLI flag.
// ---------------------------------------------------------------------------

// runAppletResizeCommand boots the resize test applet, publishes /render via
// AppletBase, and redraws whenever pane dimensions change.
func runAppletResizeCommand(apiAddr string, out io.Writer) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		cancel()
	}()

	applet := &ResizeBorderApplet{}
	initialHeight, initialWidth := resolveWidgetPaneSizeAtStartup(out)
	lines := applet.Render(initialHeight, initialWidth)
	applet.StoreSnapshot(applet.Name(), initialHeight, initialWidth, lines)

	boundAddr, err := applet.StartAPIServer(ctx, apiAddr, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[APPLET RESIZE] failed to bind API server: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "[APPLET RESIZE] API http://%s/render\n", boundAddr)

	if err := applet.RunResizeLoop(ctx, applet, out, 120*time.Millisecond); err != nil {
		fmt.Fprintf(os.Stderr, "[APPLET RESIZE] failed: %v\n", err)
		return 1
	}
	return 0
}
