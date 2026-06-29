package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"agentx/internal/session"
	"agentx/internal/surfaces"
)

// AttachError is a typed surface-attach failure carrying a deterministic reason
// category (validation | auth | transport | conflict) for the launch CLI.
type AttachError struct {
	Category string
	Message  string
}

func (e *AttachError) Error() string { return fmt.Sprintf("%s: %s", e.Category, e.Message) }

// Client attaches a surface to a running orchestrator over the loopback transport.
type Client struct {
	endpoint string
	hc       *http.Client
}

// NewClient returns an attach client for an orchestrator endpoint (e.g.
// http://127.0.0.1:8420).
func NewClient(endpoint string) *Client {
	return &Client{endpoint: endpoint, hc: http.DefaultClient}
}

// CurrentSession reads the orchestrator's active session. A connection failure is
// reported with category transport.
func (c *Client) CurrentSession(ctx context.Context) (session.Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/sessions/current", nil)
	if err != nil {
		return session.Identity{}, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return session.Identity{}, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return session.Identity{}, attachErrorFrom(resp)
	}
	var id session.Identity
	if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
		return session.Identity{}, &AttachError{Category: "transport", Message: "malformed session response"}
	}
	return id, nil
}

// Register attaches a surface of the given kind, returning the stored
// registration. A connection failure is category transport; a server rejection
// carries its own category (auth | validation | conflict).
func (c *Client) Register(ctx context.Context, kind, transportAddr, token string, capabilities []string) (surfaces.Registration, error) {
	body, _ := json.Marshal(registerBody{
		SurfaceKind:      kind,
		TransportAddress: transportAddr,
		Capabilities:     capabilities,
		Token:            token,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/surface/register", bytes.NewReader(body))
	if err != nil {
		return surfaces.Registration{}, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return surfaces.Registration{}, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return surfaces.Registration{}, attachErrorFrom(resp)
	}
	var reg surfaces.Registration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return surfaces.Registration{}, &AttachError{Category: "transport", Message: "malformed registration response"}
	}
	return reg, nil
}

// attachErrorFrom decodes a {error,category} body into an AttachError, defaulting
// the category from the status when the body is unhelpful.
func attachErrorFrom(resp *http.Response) *AttachError {
	data, _ := io.ReadAll(resp.Body)
	var payload struct {
		Error    string `json:"error"`
		Category string `json:"category"`
	}
	_ = json.Unmarshal(data, &payload)
	category := payload.Category
	if category == "" {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			category = surfaces.CategoryAuth
		case http.StatusConflict:
			category = surfaces.CategoryConflict
		default:
			category = surfaces.CategoryValidation
		}
	}
	msg := payload.Error
	if msg == "" {
		msg = fmt.Sprintf("registration failed with status %d", resp.StatusCode)
	}
	return &AttachError{Category: category, Message: msg}
}
