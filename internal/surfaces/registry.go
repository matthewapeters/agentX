// Package surfaces holds the orchestrator-side surface registry and the
// ephemeral attach-token machinery that external surface processes use to attach
// to a running session (TRN-1).
//
// The registry is the canonical record of which surfaces are attached to a
// session. It validates the per-session attach token at registration, stores the
// frozen registration payload (see
// docs/architecture/runtime_contracts/surface-registration.schema.json), and
// manages surface lifecycle. The raw attach token never leaves orchestrator
// memory; only a non-secret fingerprint is stored or published.
package surfaces

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
)

// Lifecycle states (docs/implementation/02, Surface Registration Contract).
// provisioning and degraded are reserved by the frozen schema for future use; v1
// registration transitions straight to ready.
const (
	LifecycleProvisioning = "provisioning"
	LifecycleReady        = "ready"
	LifecycleDegraded     = "degraded"
	LifecycleStopped      = "stopped"
)

// Reason categories for a rejected registration, surfaced to transport handlers
// and the launch CLI for deterministic messages/exit codes.
const (
	CategoryValidation = "validation"
	CategoryAuth       = "auth"
	CategoryConflict   = "conflict"
)

// fingerprintLen is the hex-character width of a published token fingerprint.
const fingerprintLen = 16

// Registration is the record a surface publishes when attaching. The JSON shape
// mirrors the frozen surface-registration.schema.json.
type Registration struct {
	SurfaceID              string   `json:"surface_id"`
	SurfaceKind            string   `json:"surface_kind"`
	TransportAddress       string   `json:"transport_address"`
	Capabilities           []string `json:"capabilities"`
	LifecycleState         string   `json:"lifecycle_state"`
	SessionID              string   `json:"session_id"`
	SessionName            string   `json:"session_name"`
	AttachTokenFingerprint string   `json:"attach_token_fingerprint"`
}

// RegisterRequest is the inbound payload a surface presents to attach. Token is
// the raw attach token; it is validated and discarded (never stored).
type RegisterRequest struct {
	SurfaceID        string
	SurfaceKind      string
	TransportAddress string
	Capabilities     []string
	Token            string
}

// RegisterError is a rejected registration carrying a deterministic category.
type RegisterError struct {
	Category string
	Message  string
}

func (e *RegisterError) Error() string {
	return fmt.Sprintf("%s: %s", e.Category, e.Message)
}

// AttachToken is a session's ephemeral attach token. The raw value is held only
// in memory and never persisted; only Fingerprint is publishable.
type AttachToken struct {
	raw         string
	fingerprint string
}

// MintToken generates a high-entropy ephemeral attach token.
func MintToken() (AttachToken, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return AttachToken{}, fmt.Errorf("read random: %w", err)
	}
	raw := hex.EncodeToString(buf)
	return AttachToken{raw: raw, fingerprint: fingerprint(raw)}, nil
}

// Raw returns the secret token value (for the orchestrator to print in launch
// command strings). It is never persisted.
func (t AttachToken) Raw() string { return t.raw }

// Fingerprint returns the non-secret fingerprint stored/published on registrations.
func (t AttachToken) Fingerprint() string { return t.fingerprint }

// Validate reports whether presented matches this token, in constant time. An
// empty token never validates.
func (t AttachToken) Validate(presented string) bool {
	if t.raw == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(t.raw), []byte(presented)) == 1
}

// fingerprint is the hex SHA-256 of raw, truncated to fingerprintLen.
func fingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:fingerprintLen]
}

// Registry is the orchestrator-side store of surface registrations for one
// session. It is safe for concurrent use.
type Registry struct {
	token       AttachToken
	sessionID   string
	sessionName string

	mu       sync.Mutex
	surfaces map[string]Registration
}

// NewRegistry returns a registry bound to a session and its attach token.
func NewRegistry(token AttachToken, sessionID, sessionName string) *Registry {
	return &Registry{
		token:       token,
		sessionID:   sessionID,
		sessionName: sessionName,
		surfaces:    make(map[string]Registration),
	}
}

// Register validates the presented token and records the surface as ready. A
// missing/wrong token is an auth rejection; a missing kind is a validation
// rejection; a duplicate of a non-stopped surface id is a conflict. Re-registering
// a stopped id is allowed. An empty SurfaceID is assigned a generated id.
func (r *Registry) Register(req RegisterRequest) (Registration, error) {
	if !r.token.Validate(req.Token) {
		return Registration{}, &RegisterError{Category: CategoryAuth, Message: "invalid or missing attach token"}
	}
	if req.SurfaceKind == "" {
		return Registration{}, &RegisterError{Category: CategoryValidation, Message: "surface_kind is required"}
	}
	addr := req.TransportAddress
	if addr == "" {
		return Registration{}, &RegisterError{Category: CategoryValidation, Message: "transport_address is required"}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	id := req.SurfaceID
	if id == "" {
		id = r.generateID(req.SurfaceKind)
	} else if existing, ok := r.surfaces[id]; ok && existing.LifecycleState != LifecycleStopped {
		return Registration{}, &RegisterError{Category: CategoryConflict, Message: fmt.Sprintf("surface_id %q already registered", id)}
	}

	caps := req.Capabilities
	if caps == nil {
		caps = []string{}
	}
	reg := Registration{
		SurfaceID:              id,
		SurfaceKind:            req.SurfaceKind,
		TransportAddress:       addr,
		Capabilities:           caps,
		LifecycleState:         LifecycleReady,
		SessionID:              r.sessionID,
		SessionName:            r.sessionName,
		AttachTokenFingerprint: r.token.Fingerprint(),
	}
	r.surfaces[id] = reg
	return reg, nil
}

// Shutdown transitions a registered surface to stopped. Unknown ids are a
// validation rejection.
func (r *Registry) Shutdown(surfaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.surfaces[surfaceID]
	if !ok {
		return &RegisterError{Category: CategoryValidation, Message: fmt.Sprintf("unknown surface_id %q", surfaceID)}
	}
	reg.LifecycleState = LifecycleStopped
	r.surfaces[surfaceID] = reg
	return nil
}

// ValidateToken reports whether raw matches this session's attach token. Used by
// the transport to authorize write requests from already-attached surfaces.
func (r *Registry) ValidateToken(raw string) bool { return r.token.Validate(raw) }

// Get returns the registration for an id.
func (r *Registry) Get(surfaceID string) (Registration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.surfaces[surfaceID]
	return reg, ok
}

// List returns the registrations ordered deterministically by surface_id.
func (r *Registry) List() []Registration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Registration, 0, len(r.surfaces))
	for _, reg := range r.surfaces {
		out = append(out, reg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SurfaceID < out[j].SurfaceID })
	return out
}

// generateID returns a unique id for a kind. The caller holds the lock.
func (r *Registry) generateID(kind string) string {
	buf := make([]byte, 4)
	for {
		_, _ = rand.Read(buf)
		id := fmt.Sprintf("%s-%s", kind, hex.EncodeToString(buf))
		if _, exists := r.surfaces[id]; !exists {
			return id
		}
	}
}
