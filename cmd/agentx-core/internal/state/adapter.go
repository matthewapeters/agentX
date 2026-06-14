package state

import (
	"context"
	"fmt"
)

// StateAdapter bridges between v0 (implicit index-based) and v1 (semantic-key) formats.
type StateAdapter interface {
	// ToV1 converts v0 map format to versioned applet state (v1).
	ToV1(ctx context.Context, v0 map[string]interface{}) (*VersionedAppletState, error)
	// FromV1 extracts v1 payload back to v0 map format.
	FromV1(ctx context.Context, v1 *VersionedAppletState) (map[string]interface{}, error)
}

// NewStateAdapter creates a StateAdapter for the given widget type.
// Supported widgets: "output", "context", "input".
func NewStateAdapter(ctx context.Context, widget string) (StateAdapter, error) {
	switch widget {
	case "output":
		return &outputAdapter{widget: widget}, nil
	case "context":
		return &contextAdapter{widget: widget}, nil
	case "input":
		return &inputAdapter{widget: widget}, nil
	default:
		return nil, fmt.Errorf("unsupported widget type: %q", widget)
	}
}

// outputAdapter handles v0↔v1 migration for output widget.
type outputAdapter struct {
	widget string
}

// contextAdapter handles v0↔v1 migration for context widget.
type contextAdapter struct {
	widget string
}

// inputAdapter handles v0↔v1 migration for input widget.
type inputAdapter struct {
	widget string
}
