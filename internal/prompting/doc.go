// Package prompting assembles the message sequence sent to the model.
//
// Source contract: docs/implementation/04_llm_prompt_tooling_runtime.md (Prompt
// Stack Model). Backlog task: CHT-C2. This MVP composes a system prompt plus the
// user message; persona, skills, commands, and procedural stages are deferred.
//
// It defines its own Message type rather than importing the model adapter, so
// the runtime maps prompting.Message to the adapter's request type (honoring the
// import-direction matrix in docs/implementation/08_go_module_layout.md).
package prompting
