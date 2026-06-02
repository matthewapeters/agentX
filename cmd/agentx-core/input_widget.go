package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type widgetActivitySnapshot struct {
	SessionID   string            `json:"session_id"`
	State       string            `json:"state"`
	Phase       string            `json:"phase"`
	PromptCycle PromptCycleStatus `json:"prompt_cycle"`
	ContextFiles []string         `json:"context_files,omitempty"`
}

type widgetActivityState struct {
	mu         sync.RWMutex
	state      string
	phase      string
	doneUntil  time.Time
	failUntil  time.Time
	sessionID  string
	contextFile string
}

func newWidgetActivityState() *widgetActivityState {
	return &widgetActivityState{state: "idle", phase: "none"}
}

func (ws *widgetActivityState) update(snapshot widgetActivitySnapshot) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.state = strings.TrimSpace(snapshot.State)
	ws.phase = strings.TrimSpace(snapshot.Phase)
	ws.sessionID = strings.TrimSpace(snapshot.SessionID)
	now := time.Now()
	if ws.state == "completed" {
		ws.doneUntil = now.Add(1200 * time.Millisecond)
	}
	if ws.state == "failed" {
		ws.failUntil = now.Add(2000 * time.Millisecond)
	}
	ws.contextFile = ""
	if n := len(snapshot.ContextFiles); n > 0 {
		ws.contextFile = filepath.Base(strings.TrimSpace(snapshot.ContextFiles[n-1]))
	}
}

func (ws *widgetActivityState) promptLabel() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	now := time.Now()
	contextSuffix := ""
	if ws.contextFile != "" {
		contextSuffix = fmt.Sprintf("[ctx:%s]", ws.contextFile)
	}
	if ws.state == "working" {
		if ws.phase != "" && ws.phase != "none" {
			return fmt.Sprintf("agentx[%s]%s", ws.phase, contextSuffix)
		}
		return "agentx[working]" + contextSuffix
	}
	if now.Before(ws.failUntil) {
		return "agentx[failed]" + contextSuffix
	}
	if now.Before(ws.doneUntil) {
		return "agentx[done]" + contextSuffix
	}
	return "agentx" + contextSuffix
}

func startWidgetActivityPoller(baseURL string, target *widgetActivityState) func() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollCtx, pollCancel := context.WithTimeout(ctx, 1200*time.Millisecond)
				snapshot, err := fetchWidgetActivitySnapshot(pollCtx, baseURL)
				pollCancel()
				if err != nil {
					continue
				}
				target.update(snapshot)
			}
		}
	}()

	return func() {
		cancel()
		wg.Wait()
	}
}

func runInputWidgetCommand(coreHTTP string, in io.Reader, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Input widget failed: missing core HTTP base URL")
		return 1
	}
	baseURL = strings.TrimRight(baseURL, "/")
	activityState := newWidgetActivityState()
	stopPoller := startWidgetActivityPoller(baseURL, activityState)
	defer stopPoller()
	submitTimeout := resolveWidgetSubmitTimeout()

	fmt.Fprintln(out, "Input ready. Enter prompt and press Enter.")
	fmt.Fprintln(out, "Commands: :q, :clear, :context-add <file-path> (alias :ctx-add), :help")

	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprintf(out, "%s> ", activityState.promptLabel())
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(out, "Input widget failed: %v\n", err)
				return 1
			}
			return 0
		}

		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}

		// Remove the just-submitted input line so acceptance feedback appears immediately.
		fmt.Fprint(out, "\033[1A\033[2K\r")

		switch strings.ToLower(prompt) {
		case ":help", ":commands":
			fmt.Fprintln(out, "Available commands:")
			fmt.Fprintln(out, "  :q                     Shut down the current session")
			fmt.Fprintln(out, "  :clear                 Clear input panel only")
			fmt.Fprintln(out, "  :context-add <path>    Register a context file")
			fmt.Fprintln(out, "  :ctx-add <path>        Alias for :context-add")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), submitTimeout)
		response, err := submitPromptToCore(ctx, baseURL, prompt)
		cancel()
		if err != nil {
			fmt.Fprintf(out, "Submit failed: %v\n", err)
			if prompt == ":q" {
				return 1
			}
			continue
		}

		switch prompt {
		case ":q":
			fmt.Fprintln(out, "Session shutdown requested.")
			_ = response
			return 0
		case ":clear":
			_ = response
		default:
			_ = response
		}
	}
}

func resolveWidgetSubmitTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AGENTX_SUBMIT_TIMEOUT_SEC"))
	if raw == "" {
		return 120 * time.Second
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 120 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

type submitErrorResponse struct {
	Error string `json:"error"`
}

func submitPromptToCore(ctx context.Context, baseURL string, prompt string) (string, error) {
	payload := submitRequest{Prompt: prompt}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/submit", bytes.NewReader(encodedPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorPayload submitErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errorPayload); decodeErr == nil && strings.TrimSpace(errorPayload.Error) != "" {
			return "", fmt.Errorf(errorPayload.Error)
		}
		return "", fmt.Errorf("submit failed with status %d", resp.StatusCode)
	}

	var success submitResponse
	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return "", err
	}
	if strings.TrimSpace(success.Response) == "" {
		return "", fmt.Errorf("submit endpoint returned empty response")
	}

	return success.Response, nil
}

func fetchWidgetActivitySnapshot(ctx context.Context, baseURL string) (widgetActivitySnapshot, error) {
	var snapshot widgetActivitySnapshot
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/activity", nil)
	if err != nil {
		return snapshot, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return snapshot, fmt.Errorf("activity failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return snapshot, err
	}
	if strings.TrimSpace(snapshot.State) == "" {
		snapshot.State = "idle"
	}
	if strings.TrimSpace(snapshot.Phase) == "" {
		snapshot.Phase = "none"
	}
	return snapshot, nil
}
