package runtimesteps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/llm/ollama"
)

type ollamaWorld struct {
	server     *httptest.Server
	client     *ollama.Client
	deltas     []string
	response   string
	err        error
	ctxLen     int
	sentNumCtx int // options.num_ctx captured from the last /api/chat request
}

// registerOllamaSteps wires the Ollama adapter steps (CHT-C1).
func registerOllamaSteps(sc *godog.ScenarioContext) {
	w := &ollamaWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.server != nil {
			w.server.Close()
		}
		*w = ollamaWorld{}
		return ctx, err
	})

	sc.Step(`^a stub Ollama server that streams "([^"]*)" then "([^"]*)"$`, w.serverStreams)
	sc.Step(`^a stub Ollama server listing model "([^"]*)"$`, w.serverListing)
	sc.Step(`^a stub Ollama server that returns HTTP 500$`, w.serverError)
	sc.Step(`^a stub Ollama server reporting architecture "([^"]*)" with context length (\d+)$`, w.serverShow)
	sc.Step(`^a chat request for model "([^"]*)" is sent with prompt "([^"]*)"$`, w.sendChat)
	sc.Step(`^a chat request for model "([^"]*)" is sent with prompt "([^"]*)" and context window (\d+)$`, w.sendChatNumCtx)
	sc.Step(`^readiness is checked for model "([^"]*)"$`, w.checkReady)
	sc.Step(`^the context length for model "([^"]*)" is fetched$`, w.fetchContextLength)
	sc.Step(`^the reported context length is (\d+)$`, w.contextLengthIs)
	sc.Step(`^the context length lookup fails$`, w.contextLengthFailed)
	sc.Step(`^the chat request set num_ctx to (\d+)$`, w.numCtxSent)
	sc.Step(`^the streamed deltas are "([^"]*)" then "([^"]*)"$`, w.deltasAre)
	sc.Step(`^the assembled response is "([^"]*)"$`, w.responseIs)
	sc.Step(`^readiness passes$`, w.readyPasses)
	sc.Step(`^readiness fails$`, w.readyFails)
	sc.Step(`^the chat returns an error$`, w.chatErrored)
}

func (w *ollamaWorld) start(handler http.Handler) {
	w.server = httptest.NewServer(handler)
	w.client = ollama.New(w.server.URL)
}

func (w *ollamaWorld) serverStreams(first, second string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(rw http.ResponseWriter, _ *http.Request) {
		enc := json.NewEncoder(rw)
		_ = enc.Encode(chunk(first, false))
		_ = enc.Encode(chunk(second, false))
		_ = enc.Encode(map[string]any{"done": true})
	})
	w.start(mux)
	return nil
}

func (w *ollamaWorld) serverListing(model string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(rw http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"models": []map[string]string{{"name": model}},
		})
	})
	w.start(mux)
	return nil
}

func (w *ollamaWorld) serverError() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	})
	w.start(mux)
	return nil
}

// serverShow stubs POST /api/show, reporting the model's architecture and its
// per-architecture context length (mirroring Ollama's model_info shape).
func (w *ollamaWorld) serverShow(arch string, length int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/show", func(rw http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"model_info": map[string]any{
				"general.architecture":    arch,
				arch + ".context_length": length,
			},
		})
	})
	w.start(mux)
	return nil
}

func (w *ollamaWorld) sendChat(model, prompt string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.response, w.err = w.client.Chat(ctx, ollama.ChatRequest{
		Model:    model,
		Messages: []ollama.Message{{Role: "user", Content: prompt}},
	}, func(d string) { w.deltas = append(w.deltas, d) }, nil)
	return nil
}

// sendChatNumCtx sends a chat with an explicit context window and captures the
// options.num_ctx the server received, proving the window reaches Ollama.
func (w *ollamaWorld) sendChatNumCtx(model, prompt string, numCtx int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Options struct {
				NumCtx int `json:"num_ctx"`
			} `json:"options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.sentNumCtx = body.Options.NumCtx
		_ = json.NewEncoder(rw).Encode(map[string]any{"done": true})
	})
	w.start(mux)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.response, w.err = w.client.Chat(ctx, ollama.ChatRequest{
		Model:    model,
		Messages: []ollama.Message{{Role: "user", Content: prompt}},
		NumCtx:   numCtx,
	}, func(string) {}, nil)
	return nil
}

func (w *ollamaWorld) fetchContextLength(model string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.ctxLen, w.err = w.client.ContextLength(ctx, model)
	return nil
}

func (w *ollamaWorld) contextLengthIs(want int) error {
	if w.ctxLen != want {
		return fmt.Errorf("context length = %d, want %d", w.ctxLen, want)
	}
	return nil
}

func (w *ollamaWorld) contextLengthFailed() error {
	if w.err == nil {
		return fmt.Errorf("expected context length lookup to fail")
	}
	return nil
}

func (w *ollamaWorld) numCtxSent(want int) error {
	if w.sentNumCtx != want {
		return fmt.Errorf("num_ctx = %d, want %d", w.sentNumCtx, want)
	}
	return nil
}

func (w *ollamaWorld) checkReady(model string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.err = w.client.Ready(ctx, model)
	return nil
}

func (w *ollamaWorld) deltasAre(first, second string) error {
	if len(w.deltas) != 2 || w.deltas[0] != first || w.deltas[1] != second {
		return fmt.Errorf("deltas = %v, want [%q %q]", w.deltas, first, second)
	}
	return nil
}

func (w *ollamaWorld) responseIs(want string) error {
	if w.response != want {
		return fmt.Errorf("response = %q, want %q", w.response, want)
	}
	return nil
}

func (w *ollamaWorld) readyPasses() error {
	if w.err != nil {
		return fmt.Errorf("expected readiness to pass, got %v", w.err)
	}
	return nil
}

func (w *ollamaWorld) readyFails() error {
	if w.err == nil {
		return fmt.Errorf("expected readiness to fail")
	}
	return nil
}

func (w *ollamaWorld) chatErrored() error {
	if w.err == nil {
		return fmt.Errorf("expected chat to return an error")
	}
	return nil
}

func chunk(content string, done bool) map[string]any {
	return map[string]any{
		"message": map[string]string{"role": "assistant", "content": content},
		"done":    done,
	}
}
