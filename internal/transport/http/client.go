package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"agentx/internal/session"
	"agentx/internal/state"
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

// Seed fetches the persisted session event log — the durable snapshot a surface
// renders before resuming the live stream (SS-1).
func (c *Client) Seed(ctx context.Context) ([]state.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/sessions/current/events", nil)
	if err != nil {
		return nil, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, attachErrorFrom(resp)
	}
	var events []state.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, &AttachError{Category: "transport", Message: "malformed seed response"}
	}
	return events, nil
}

// Subscribe opens the live event stream after the given cursor and delivers events
// on the returned channel until ctx is canceled or the stream ends (SS-1). Pass the
// last ordinal from Seed to resume with no gap or duplicate; pass 0 for the full
// stream.
// surfaceID, when non-empty, is reported to the orchestrator so it can track this
// stream as a live connection for presence/status (SS-4); pass "" to attach without
// being tracked.
func (c *Client) Subscribe(ctx context.Context, after uint64, surfaceID string) (<-chan state.Event, error) {
	endpoint := fmt.Sprintf("%s/events?after=%s", c.endpoint, strconv.FormatUint(after, 10))
	if surfaceID != "" {
		endpoint += "&surface_id=" + url.QueryEscape(surfaceID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, attachErrorFrom(resp)
	}

	ch := make(chan state.Event, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data:")
			if !ok {
				continue
			}
			var ev state.Event
			if json.Unmarshal([]byte(strings.TrimSpace(data)), &ev) != nil {
				continue
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Shutdown marks the surface stopped on the orchestrator (lifecycle → stopped),
// authorized by the attach token. Called when a surface quits (SS-2).
func (c *Client) Shutdown(ctx context.Context, surfaceID, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/surface/"+surfaceID+"/shutdown", nil)
	if err != nil {
		return &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return attachErrorFrom(resp)
	}
	return nil
}

// WorkingMemory fetches the session's working-memory facts (GET /working-memory).
func (c *Client) WorkingMemory(ctx context.Context) ([]session.Fact, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/working-memory", nil)
	if err != nil {
		return nil, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, attachErrorFrom(resp)
	}
	var body struct {
		Facts []session.Fact `json:"facts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, &AttachError{Category: "transport", Message: err.Error()}
	}
	return body.Facts, nil
}

// ContextBreakdown fetches the assembled context window's composition by content
// class (GET /context) for the read-only context-visualizer surface.
func (c *Client) ContextBreakdown(ctx context.Context) (session.ContextReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/context", nil)
	if err != nil {
		return session.ContextReport{}, &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return session.ContextReport{}, &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return session.ContextReport{}, attachErrorFrom(resp)
	}
	var report session.ContextReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return session.ContextReport{}, &AttachError{Category: "transport", Message: err.Error()}
	}
	return report, nil
}

// SetEventEnabled toggles whether a conversation element (by ordinal) folds into
// the agent's upcoming context (POST /events/{ordinal}/enabled).
func (c *Client) SetEventEnabled(ctx context.Context, token string, ordinal uint64, enabled bool) error {
	path := fmt.Sprintf("/events/%d/enabled", ordinal)
	return c.postWM(ctx, token, path, map[string]any{"enabled": enabled})
}

// SetFact adds or edits a working-memory fact (POST /working-memory/set).
func (c *Client) SetFact(ctx context.Context, token, key, value string) error {
	return c.postWM(ctx, token, "/working-memory/set", map[string]any{"key": key, "value": value})
}

// DeleteFact removes a working-memory fact (POST /working-memory/delete).
func (c *Client) DeleteFact(ctx context.Context, token, key string) error {
	return c.postWM(ctx, token, "/working-memory/delete", map[string]any{"key": key})
}

// SetFactEnabled enables or disables a fact (POST /working-memory/enabled).
func (c *Client) SetFactEnabled(ctx context.Context, token, key string, enabled bool) error {
	return c.postWM(ctx, token, "/working-memory/enabled", map[string]any{"key": key, "enabled": enabled})
}

// SetFactLive toggles a pinned fact's live/static state (POST /working-memory/live).
func (c *Client) SetFactLive(ctx context.Context, token, key string, live bool) error {
	return c.postWM(ctx, token, "/working-memory/live", map[string]any{"key": key, "live": live})
}

// PinToolEvent copies a tool_result conversation element (by ordinal) into
// working memory as a durable fact and disables the source event in context
// (POST /events/{ordinal}/pin). live requests a fact that re-runs its source
// tool before each turn (refused server-side if the tool is not currently
// permitted without approval); false pins a frozen snapshot. Returns the new
// fact's key.
func (c *Client) PinToolEvent(ctx context.Context, token string, ordinal uint64, live bool) (string, error) {
	data, _ := json.Marshal(map[string]any{"live": live})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/events/%d/pin", c.endpoint, ordinal), bytes.NewReader(data))
	if err != nil {
		return "", &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", attachErrorFrom(resp)
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", &AttachError{Category: "transport", Message: err.Error()}
	}
	return body.Key, nil
}

// PinPlanNode copies a plan node's own resolved value (no backing tool call — a
// Step, e.g. a wavefront Know) into working memory as a durable fact (POST
// /plans/{root}/nodes/{node}/pin, ADR 0012 amendment). Unlike PinToolEvent there
// is no live option: a Source-less fact has nothing to re-run. Returns the new
// fact's key.
func (c *Client) PinPlanNode(ctx context.Context, token, root, nodeID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/plans/%s/nodes/%s/pin", c.endpoint, root, nodeID), nil)
	if err != nil {
		return "", &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", attachErrorFrom(resp)
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", &AttachError{Category: "transport", Message: err.Error()}
	}
	return body.Key, nil
}

// postWM posts a token-authorized working-memory mutation.
func (c *Client) postWM(ctx context.Context, token, path string, payload any) error {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return &AttachError{Category: surfaces.CategoryValidation, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return &AttachError{Category: "transport", Message: fmt.Sprintf("cannot reach %s: %v", c.endpoint, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return attachErrorFrom(resp)
	}
	return nil
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
