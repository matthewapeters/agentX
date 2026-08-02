package runtime

import (
	"context"
	"fmt"
	"strings"

	"agentx/internal/prompting"
	"agentx/internal/tools"
)

// buildTools constructs the registry, policy, executor, and output-overrides
// collaborators the native tool-calling loop and plan_task both depend on.
// Called under o.mu during Start when tools are enabled.
func (o *Orchestrator) buildTools() error {
	if o.registry == nil {
		o.registry = tools.DefaultRegistry()
	}
	if o.policy == nil {
		blacklist, err := tools.LoadBlacklist(o.settings.ToolBlacklistPath)
		if err != nil {
			return err
		}
		o.policy = tools.NewPolicy(blacklist...)
		approvals, err := tools.LoadApprovals(o.settings.ToolApprovalsPath)
		if err != nil {
			return err
		}
		o.policy.LoadGlobal(approvals)
	}
	if o.runner == nil {
		art, err := o.store.Artifacts(o.id.ID)
		if err != nil {
			return fmt.Errorf("tool artifacts: %w", err)
		}
		o.runner = tools.NewExecutor(art, o.settings.ToolOutputMaxBytes)
	}
	if o.outputOverrides == nil {
		overrides, err := tools.LoadOutputOverrides(o.settings.ToolOutputOverridesPath)
		if err != nil {
			return err
		}
		o.outputOverrides = tools.NewOutputOverrides(overrides)
	}
	return nil
}

// toolsReady reports whether native tool-calling can advertise/execute tools.
// Kept on Orchestrator (not unified with ConversationCore.toolsReady in
// core_tools.go) because classifier_pipeline.go's disconnected pipeline still
// calls this copy directly — see core_tools.go's doc comment.
func (o *Orchestrator) toolsReady() bool {
	return o.settings.ToolsEnabled && o.runner != nil && o.policy != nil && o.registry != nil
}

// availableToolSchemas returns every tool schema this turn should advertise to
// the model: Core's native generic catalog (core_tools.go's toolSchemas, ADR
// 0013 Phase 3) plus plan_task when the decomposition substrate is wired —
// plan_task stays an Orchestrator-only concern (ADR 0013 §"Explicitly not
// decided here"), so this thin wrapper is what buildCore binds into Core's
// toolSchemasFn closure, not a fully-native Core method.
func (o *Orchestrator) availableToolSchemas() []tools.ToolSchema {
	out := o.core.toolSchemas()
	if o.planReady() {
		out = append(out, planTaskSchema())
	}
	return out
}

// toolPin carries the ordinals and rendered text of the tool_call/tool_result
// events one native tool call published, so recordTurn can register them as
// toggleable context-history entries (initially disabled, like their
// checkbox) alongside the turn's user/assistant elements. An entry with
// ordinal 0 was never published (e.g. the approval-gated path publishes no
// separate 🔧 tool_call widget — the approval request/decision audit trail
// stands in for it) and is skipped by recordTurn.
type toolPin struct {
	callOrdinal   uint64
	callText      string
	resultOrdinal uint64
	resultText    string
}

// streamResponse and finishCycle are thin wrappers over ConversationCore's
// native implementations (core_respond.go, ADR 0013 Phase 3) — kept on
// Orchestrator because continuation.go and classifier_pipeline.go's
// disconnected pipeline still call these two directly.
func (o *Orchestrator) streamResponse(ctx context.Context, messages, fallback []prompting.Message, toolSchemas []tools.ToolSchema, doThink, ephemeral bool) (ChatResult, uint64, error) {
	return o.core.streamResponse(ctx, messages, fallback, toolSchemas, doThink, ephemeral)
}

func (o *Orchestrator) finishCycle(err error) error {
	return o.core.finishCycle(err)
}

func toolResultText(res tools.Result) string {
	head := fmt.Sprintf("[%s exit=%d, %d lines]", res.Status, res.Exit, res.Lines)
	if res.Preview == "" {
		return head
	}
	return head + "\n" + res.Preview
}

// toolResultContext renders the tool outcome as the content of a native
// tool-result message: the full captured result text (never a truncated
// excerpt of it — see tools.Result), plus a ref for re-reading a specific
// window via read_output.
func toolResultContext(d tools.Descriptor, res tools.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[tool %s result — status %s, exit %d, %d lines]\n%s",
		d.ID, res.Status, res.Exit, res.Lines, res.Preview)
	if res.Ref != "" {
		fmt.Fprintf(&b, "\n(artifact ref: %s; use read_output to re-read a specific window)", res.Ref)
	}
	return b.String()
}

func toolDeniedContext(d tools.Descriptor, reason string) string {
	return fmt.Sprintf("[tool %s was not permitted: %s]", d.ID, reason)
}
