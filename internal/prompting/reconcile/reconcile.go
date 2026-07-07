// Package reconcile folds the turn classification ([B]: did the user request an
// action?) and the response classification ([C]: did the model produce or execute
// one?) into a single route ([D] in cascade_classifier.md). Neither signal decides
// alone: the reconciler is where the vivid-willow case — the model committed to an
// action in prose but never executed it — is caught and routed to reification.
//
// It is a pure fold over two signal structs, decoupled from how the signals are
// produced (a future adapter maps fanout.Decision / task records onto them), so the
// routing table is testable in isolation.
//
// Design: docs/architecture/cascade_classifier.md (§ Reconciliation).
// Behavior contract: tests/features/prompting/reconcile.feature.
package reconcile

// TurnSignal is the turn classifier's read: did the user ask for an action?
type TurnSignal struct {
	Actionable bool
	Abstained  bool // classification too uncertain to trust
}

// ResponseSignal is the response classifier's read of what the model actually did.
type ResponseSignal struct {
	Produced  bool // the response contains an artifact/action it committed to
	Executed  bool // the action was actually carried out
	Abstained bool
	// LeansProduced marks an abstained response whose vote scattered toward
	// produced/executed rather than "none" — the model narrated a multi-step action it
	// could not resolve to a single call. It discriminates Decompose (a compound action,
	// reify a plan) from Ask (genuine ambiguity). Only meaningful when Abstained is true.
	LeansProduced bool
}

// Route is the reconciled decision the executor stage acts on.
type Route string

const (
	// Verify — the model executed the action; confirm the effect and finish.
	Verify Route = "verify"
	// Reify — the model produced the artifact in prose but never executed it (the
	// vivid-willow case); reify it into a real tool call.
	Reify Route = "reify"
	// Redispatch — the user asked for an action the model dropped entirely;
	// dispatch the task record to the executor.
	Redispatch Route = "redispatch"
	// Confirm — the model volunteered an action the user never asked for; gate it
	// through the command policy / confirmation.
	Confirm Route = "confirm"
	// None — pure conversation; no task.
	None Route = "none"
	// Ask — a classifier abstained; ask one clarifying question rather than guess.
	Ask Route = "ask"
	// Decompose — the turn is actionable but the model narrated a multi-step action it
	// could not execute in one call (the response scattered toward produced). It is a
	// compound goal: reify a *plan* (decompose into a task DAG), not a single tool call.
	// The generalization of Reify from one call to a plan. See ADR 0008.
	Decompose Route = "decompose"
)

// Reconcile folds the two signals into a route. An abstain on either side routes
// to Ask (accuracy-first: never guess a write or command). Otherwise the turn
// decides whether an action was requested and the response decides what became of
// it.
func Reconcile(turn TurnSignal, resp ResponseSignal) (Route, string) {
	// An actionable, non-abstained turn whose response scattered toward "produced" is not
	// uncertain — the model narrated a multi-step action. Reify a plan: Decompose. This is
	// checked before the abstain→Ask fallthrough, since the response abstained here.
	if turn.Actionable && !turn.Abstained && resp.Abstained && resp.LeansProduced {
		return Decompose, "compound action narrated across steps"
	}
	if turn.Abstained || resp.Abstained {
		return Ask, "classification uncertain"
	}
	if turn.Actionable {
		switch {
		case resp.Executed:
			return Verify, "action requested and executed"
		case resp.Produced:
			return Reify, "action produced in prose but not executed"
		default:
			return Redispatch, "action requested but dropped"
		}
	}
	if resp.Produced || resp.Executed {
		return Confirm, "action volunteered without a request"
	}
	return None, "pure conversation"
}
