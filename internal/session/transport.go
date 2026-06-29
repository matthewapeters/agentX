package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TransportInfo is the discoverable transport metadata for a session, persisted
// as sessions/<id>/transport.json (TRN-4). It carries the endpoint external
// surfaces attach to; the raw attach token is never written here.
type TransportInfo struct {
	SessionID string `json:"session_id"`
	Endpoint  string `json:"endpoint"`
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
