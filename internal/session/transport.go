package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TransportInfo is the discoverable transport metadata for a session, persisted
// as sessions/<id>/transport.json (TRN-4). It carries the endpoint external
// surfaces attach to; the raw attach token is never written here.
type TransportInfo struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Endpoint    string `json:"endpoint"`
}

// transportFile is the per-session transport metadata filename.
const transportFile = "transport.json"

// WriteTransport publishes the transport endpoint to the session's metadata.
func (s *Store) WriteTransport(id string, info TransportInfo) error {
	return writeJSON(filepath.Join(s.Dir(id), transportFile), info)
}

// ReadTransport reads the published transport metadata for a session.
func (s *Store) ReadTransport(id string) (TransportInfo, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir(id), transportFile))
	if err != nil {
		return TransportInfo{}, fmt.Errorf("read transport metadata: %w", err)
	}
	var info TransportInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return TransportInfo{}, fmt.Errorf("decode transport metadata: %w", err)
	}
	return info, nil
}

// attachTokenFile is the per-session attach-token filename. It holds the raw token
// at mode 0600 so a same-machine peer launch can resolve it from disk without the
// user copying it (SS-5). It is the only place the raw token is persisted.
const attachTokenFile = "attach-token"

// WriteAttachToken persists the raw attach token for a session at mode 0600.
func (s *Store) WriteAttachToken(id, raw string) error {
	if err := os.WriteFile(filepath.Join(s.Dir(id), attachTokenFile), []byte(raw+"\n"), 0o600); err != nil {
		return fmt.Errorf("write attach token: %w", err)
	}
	return nil
}

// ReadAttachToken reads the persisted attach token for a session.
func (s *Store) ReadAttachToken(id string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir(id), attachTokenFile))
	if err != nil {
		return "", fmt.Errorf("read attach token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// RemoveAttachToken deletes the persisted attach token (orchestrator shutdown). A
// missing file is not an error.
func (s *Store) RemoveAttachToken(id string) error {
	if err := os.Remove(filepath.Join(s.Dir(id), attachTokenFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove attach token: %w", err)
	}
	return nil
}

// ActiveTransport is a discovered session endpoint plus its attach token, used by a
// peer launch to resolve connection details from disk (SS-5).
type ActiveTransport struct {
	SessionID   string
	SessionName string
	Endpoint    string
	Token       string
	ModTime     time.Time
}

// DiscoverTransports returns every session that has published both a transport
// endpoint and a readable attach token, newest first (by transport.json mtime).
// Loopback-only v1 normally has exactly one active session.
func (s *Store) DiscoverTransports() ([]ActiveTransport, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session root: %w", err)
	}
	var out []ActiveTransport
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		info, err := s.ReadTransport(id)
		if err != nil || info.Endpoint == "" {
			continue
		}
		tok, err := s.ReadAttachToken(id)
		if err != nil || tok == "" {
			continue
		}
		var mt time.Time
		if fi, err := os.Stat(filepath.Join(s.Dir(id), transportFile)); err == nil {
			mt = fi.ModTime()
		}
		out = append(out, ActiveTransport{SessionID: info.SessionID, SessionName: info.SessionName, Endpoint: info.Endpoint, Token: tok, ModTime: mt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}
