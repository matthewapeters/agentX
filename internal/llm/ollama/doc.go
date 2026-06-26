// Package ollama is the default LLM adapter: it streams chat completions from a
// local Ollama runtime over HTTP and probes model readiness.
//
// Source contract: docs/implementation/04_llm_prompt_tooling_runtime.md (Default
// Model Service). Backlog task: CHT-C1.
//
// Behavior (GIVEN/WHEN/THEN):
//
//	GIVEN a reachable Ollama host and an available model
//	WHEN  Chat is called with a request
//	THEN  it streams assistant content deltas to the callback, returns the
//	      assembled response, and honors context cancellation (stop).
//
//	GIVEN an Ollama host
//	WHEN  Ready is called for a model
//	THEN  it succeeds only if the host is reachable and the model is listed.
package ollama
