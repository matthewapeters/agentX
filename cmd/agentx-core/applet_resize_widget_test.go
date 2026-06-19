package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ResizeBorderApplet pure renderer tests
// ---------------------------------------------------------------------------

func TestBuildResizeBorderLines_RectangularFrame(t *testing.T) {
	lines := buildResizeBorderLines(4, 6)
	expected := []string{
		"+----+",
		"|    |",
		"|    |",
		"+----+",
	}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("unexpected frame lines: got=%v want=%v", lines, expected)
	}
}

func TestBuildResizeBorderLines_OneColumn(t *testing.T) {
	lines := buildResizeBorderLines(3, 1)
	expected := []string{"+", "|", "+"}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("unexpected one-column frame: got=%v want=%v", lines, expected)
	}
}

func TestResizeBorderApplet_ImplementsAppletInterface(t *testing.T) {
	var _ Applet = (*ResizeBorderApplet)(nil) // compile-time assertion
}

func TestResizeBorderApplet_RenderMatchesBuildLines(t *testing.T) {
	a := &ResizeBorderApplet{}
	got := a.Render(5, 8)
	want := buildResizeBorderLines(5, 8)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Render mismatch: got=%v want=%v", got, want)
	}
}

// ---------------------------------------------------------------------------
// AppletBase snapshot and API tests
// ---------------------------------------------------------------------------

func TestAppletBase_StoreSnapshot_IncrementsSequence(t *testing.T) {
	var b AppletBase
	s1 := b.StoreSnapshot("test", 5, 10, []string{"a", "b"})
	s2 := b.StoreSnapshot("test", 5, 10, []string{"c", "d"})
	if s2.Sequence != s1.Sequence+1 {
		t.Fatalf("expected sequence to increment: got %d after %d", s2.Sequence, s1.Sequence)
	}
}

func TestAppletBase_StoreSnapshot_JoinsFrame(t *testing.T) {
	var b AppletBase
	snap := b.StoreSnapshot("test", 2, 5, []string{"hello", "world"})
	if snap.Frame != "hello\nworld" {
		t.Fatalf("unexpected frame: %q", snap.Frame)
	}
}

func TestAppletBase_StoreSnapshot_AppletNameCarried(t *testing.T) {
	var b AppletBase
	snap := b.StoreSnapshot("my-applet", 1, 1, []string{})
	if snap.AppletName != "my-applet" {
		t.Fatalf("expected applet_name=my-applet, got %q", snap.AppletName)
	}
}

func TestAppletBase_RenderAPIHandler_IncludesExactStrings(t *testing.T) {
	a := &ResizeBorderApplet{}
	lines := a.Render(3, 5)
	a.StoreSnapshot(a.Name(), 3, 5, lines)

	// Build handler directly from AppletBase
	mux := http.NewServeMux()
	mux.HandleFunc("/render", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.Snapshot())
	})

	req := httptest.NewRequest(http.MethodGet, "/render", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload AppletRenderSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode render payload: %v", err)
	}

	expectedLines := []string{"+---+", "|   |", "+---+"}
	if !reflect.DeepEqual(payload.Lines, expectedLines) {
		t.Fatalf("unexpected payload lines: got=%v want=%v", payload.Lines, expectedLines)
	}
	if payload.Frame != strings.Join(expectedLines, "\n") {
		t.Fatalf("unexpected payload frame: %q", payload.Frame)
	}
	if payload.Height != 3 || payload.Width != 5 {
		t.Fatalf("unexpected dimensions: height=%d width=%d", payload.Height, payload.Width)
	}
	if payload.Sequence < 1 {
		t.Fatalf("expected sequence>=1, got %d", payload.Sequence)
	}
	if payload.AppletName != "resize-border" {
		t.Fatalf("expected applet_name=resize-border, got %q", payload.AppletName)
	}
}
