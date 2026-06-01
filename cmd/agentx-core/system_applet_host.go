package main

import "strconv"

type SystemApplet interface {
	ID() string
	RenderCore(ctx SystemAppletCoreContext) []string
	RenderWidget(ctx SystemAppletWidgetContext) []string
}

type SystemAppletHost interface {
	Resolve(tab string) (SystemApplet, bool)
}

type SystemAppletCoreContext struct {
	SessionDir string
	SessionID string
	TurnCount int
	Turns     []ChatTurn
}

type SystemAppletWidgetContext struct {
	SessionDir string
	SessionID string
	TurnCount int
	Turns     []ChatTurn
}

type systemAppletRegistry struct {
	applets map[string]SystemApplet
}

func newSystemAppletHost() SystemAppletHost {
	registry := &systemAppletRegistry{applets: make(map[string]SystemApplet)}
	registry.register(contextHistorySystemApplet{})
	registry.register(workingMemorySystemApplet{})
	return registry
}

func (r *systemAppletRegistry) register(applet SystemApplet) {
	if r == nil || applet == nil {
		return
	}
	r.applets[applet.ID()] = applet
}

func (r *systemAppletRegistry) Resolve(tab string) (SystemApplet, bool) {
	if r == nil {
		return nil, false
	}
	applet, ok := r.applets[normalizeSystemTab(tab)]
	return applet, ok
}

type contextHistorySystemApplet struct{}

func (contextHistorySystemApplet) ID() string {
	return "context-history"
}

func (contextHistorySystemApplet) RenderCore(ctx SystemAppletCoreContext) []string {
	return renderContextHistoryAppletSection(ctx.TurnCount, ctx.Turns)
}

func (contextHistorySystemApplet) RenderWidget(ctx SystemAppletWidgetContext) []string {
	return renderContextHistoryAppletSection(ctx.TurnCount, ctx.Turns)
}

func renderContextHistoryAppletSection(turnCount int, turns []ChatTurn) []string {
	if turnCount == 0 {
		turnCount = len(turns)
	}
	recentPrompt := "none"
	recentResponse := "none"
	if len(turns) > 1 {
		recentPrompt = trimSingleLine(turns[len(turns)-2].Prompt, 64)
		recentResponse = trimSingleLine(turns[len(turns)-2].Response, 64)
	} else if len(turns) == 1 {
		recentPrompt = trimSingleLine(turns[0].Prompt, 64)
		recentResponse = trimSingleLine(turns[0].Response, 64)
	}

	return []string{
		"== CONTEXT HISTORY ==",
		fmtKV("history_context_count", strconv.Itoa(turnCount)),
		fmtKV("recent_prompt", recentPrompt),
		fmtKV("recent_response", recentResponse),
	}
}

func fmtKV(key string, value string) string {
	return key + ": " + value
}