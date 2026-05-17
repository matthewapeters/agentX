// Package main IPC (inter-process communication) infrastructure.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// IPCRouter manages named FIFOs for bidirectional communication with applets.
type IPCRouter struct {
	sessionID string
	fifosDir  string
}

// NewIPCRouter creates a new IPC router.
func NewIPCRouter(sessionID string, projectDir string) *IPCRouter {
	fifosDir := filepath.Join(projectDir, "sessions", "ipc", sessionID)
	return &IPCRouter{
		sessionID: sessionID,
		fifosDir:  fifosDir,
	}
}

// CreateFIFOPair creates input and output FIFOs for an applet.
func (r *IPCRouter) CreateFIFOPair(appletName string) (inputFIFO, outputFIFO string, err error) {
	// Ensure IPC directory exists.
	if err := os.MkdirAll(r.fifosDir, 0755); err != nil {
		return "", "", err
	}

	inputFIFO = filepath.Join(r.fifosDir, fmt.Sprintf("%s_input.fifo", appletName))
	outputFIFO = filepath.Join(r.fifosDir, fmt.Sprintf("%s_output.fifo", appletName))

	// Create FIFOs if they don't exist.
	for _, fifo := range []string{inputFIFO, outputFIFO} {
		if _, err := os.Stat(fifo); os.IsNotExist(err) {
			if err := os.MkFifo(fifo, 0666); err != nil {
				return "", "", err
			}
		}
	}

	return inputFIFO, outputFIFO, nil
}

// AppletMessage represents a message exchanged between core and applet.
type AppletMessage struct {
	Type    string            `json:"type"`    // "ready", "output", "input", "error"
	Payload map[string]interface{} `json:"payload"` // Message data
}

// AppletReadySignal indicates an applet has started successfully.
type AppletReadySignal struct {
	AppletName string `json:"applet_name"`
	PaneName   string `json:"pane_name"`
	Timestamp  int64  `json:"timestamp"`
}
