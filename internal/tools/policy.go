// Package tools implements AgentX's curated, policy-gated command tools.
//
// TOOL-1 covers the descriptor contract and the command policy: argument-schema
// validation plus the three-set evaluation (blacklist, global whitelist, session
// whitelist) in the order specified by
// docs/implementation/05_security_approvals_and_command_policy.md. Execution and
// the session artifact store land in TOOL-2; the approval round-trip in TOOL-3.
package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RiskLevel tiers tools by blast radius; tiers are enabled progressively.
type RiskLevel string

const (
	RiskRead    RiskLevel = "read"
	RiskWrite   RiskLevel = "write"
	RiskNetwork RiskLevel = "network"
)

// ArgKind constrains an argument's value.
type ArgKind int

const (
	// KindString is arbitrary text (content, patch, url, sed script, ...).
	KindString ArgKind = iota
	// KindInt is integer text.
	KindInt
	// KindPath is a filesystem path, guarded against option injection and
	// control characters.
	KindPath
)

// ArgSpec describes one named argument a tool accepts.
type ArgSpec struct {
	Name     string
	Kind     ArgKind
	Required bool
}

// Descriptor is a curated tool the agent may invoke (the command descriptor of
// docs/implementation/05). A descriptor with a non-empty Builtin is handled in Go;
// otherwise it executes Argv as a vector (no shell), with "{name}" tokens replaced
// by validated argument values.
type Descriptor struct {
	ID               string
	Description      string // model-facing summary; rendered into ToolSchemas' native tool-calling schema
	Command          string   // backing argv[0]; "" => Go built-in
	Argv             []string // argv template; "{name}" tokens are arg placeholders
	StdinArg         string   // arg whose value is piped to stdin ("" => none)
	Builtin          string   // non-empty => Go built-in handler key
	Args             []ArgSpec
	Risk             RiskLevel
	RequiresApproval bool
	TimeoutSeconds   int
}

// ToolSchema is a provider-agnostic native tool-calling schema entry: a name, a
// model-facing description, and its arguments as JSON Schema. internal/llm/* must
// not import this package (import-direction matrix), so internal/runtime maps
// ToolSchema to each provider's own wire-format Tool type.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// BuildArgv renders the descriptor's argv template against args, substituting
// "{name}" tokens with their (already validated) values.
func (d Descriptor) BuildArgv(args map[string]string) ([]string, error) {
	out := make([]string, 0, len(d.Argv))
	for _, tok := range d.Argv {
		if name, ok := placeholder(tok); ok {
			v, present := args[name]
			if !present {
				return nil, fmt.Errorf("argv references missing argument %q", name)
			}
			out = append(out, v)
			continue
		}
		out = append(out, tok)
	}
	return out, nil
}

func placeholder(tok string) (string, bool) {
	if len(tok) >= 2 && tok[0] == '{' && tok[len(tok)-1] == '}' {
		return tok[1 : len(tok)-1], true
	}
	return "", false
}

// Validate checks args against the descriptor schema, returning an error when an
// argument is unknown, missing, or malformed. It guards path arguments against
// option injection (leading '-') and control characters so no shell-escaping or
// flag-injection is possible in the argv/no-shell execution model.
func (d Descriptor) Validate(args map[string]string) error {
	specs := make(map[string]ArgSpec, len(d.Args))
	for _, s := range d.Args {
		specs[s.Name] = s
	}
	for name := range args {
		if _, ok := specs[name]; !ok {
			return fmt.Errorf("unknown argument %q", name)
		}
	}
	for _, s := range d.Args {
		v, present := args[s.Name]
		if !present || v == "" {
			if s.Required {
				return fmt.Errorf("missing required argument %q", s.Name)
			}
			continue
		}
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("argument %q contains a NUL byte", s.Name)
		}
		switch s.Kind {
		case KindInt:
			if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return fmt.Errorf("argument %q must be an integer", s.Name)
			}
		case KindPath:
			if strings.ContainsAny(v, "\n\r") {
				return fmt.Errorf("argument %q (path) contains a newline", s.Name)
			}
			if strings.HasPrefix(v, "-") {
				return fmt.Errorf("argument %q (path) may not begin with '-'", s.Name)
			}
		}
	}
	return nil
}

// Decision is the outcome of a policy evaluation.
type Decision int

const (
	// Allow permits execution.
	Allow Decision = iota
	// Deny blocks execution.
	Deny
	// NeedsApproval requires an interactive approval before execution.
	NeedsApproval
)

// Reason codes attached to a Verdict (AC-M3b-1: policy reason codes).
const (
	ReasonSchema    = "schema_violation"
	ReasonBlacklist = "blacklisted"
	ReasonApproval  = "approval_required"
)

// Verdict is the result of Policy.Evaluate.
type Verdict struct {
	Decision Decision
	Reason   string
}

// Rule is a blacklist entry. It applies to a tool by ID (or "*" for any). A nil
// Pattern denies the tool unconditionally; otherwise it denies when any argument
// value matches Pattern.
type Rule struct {
	Tool    string
	Pattern *regexp.Regexp
	Reason  string
}

// Scope is an approval persistence scope.
type Scope int

const (
	// ScopeSession approves for the current session only (in-memory).
	ScopeSession Scope = iota
	// ScopeGlobal approves for all sessions (persisted by TOOL-5).
	ScopeGlobal
)

// ApprovalEntry is a persisted global approval: a tool id and the validated args
// it was approved for.
type ApprovalEntry struct {
	Tool string            `toml:"tool"`
	Args map[string]string `toml:"args"`
}

// Policy holds the three documented policy sets and evaluates in order:
// blacklist → global whitelist → session whitelist → approval. The global
// whitelist persists across sessions (TOOL-5); the session set is in-memory.
type Policy struct {
	blacklist []Rule
	global    map[string]ApprovalEntry
	session   map[string]bool
}

// NewPolicy returns a policy seeded with the given blacklist rules.
func NewPolicy(blacklist ...Rule) *Policy {
	return &Policy{
		blacklist: blacklist,
		global:    map[string]ApprovalEntry{},
		session:   map[string]bool{},
	}
}

// AddRule appends a blacklist rule.
func (p *Policy) AddRule(r Rule) { p.blacklist = append(p.blacklist, r) }

// Approve records an approval for the given scope, keyed by tool + validated args.
func (p *Policy) Approve(scope Scope, d Descriptor, args map[string]string) {
	key := approvalKey(d.ID, args)
	if scope == ScopeGlobal {
		p.global[key] = ApprovalEntry{Tool: d.ID, Args: cloneArgs(args)}
		return
	}
	p.session[key] = true
}

// LoadGlobal seeds the global whitelist from persisted entries.
func (p *Policy) LoadGlobal(entries []ApprovalEntry) {
	for _, e := range entries {
		p.global[approvalKey(e.Tool, e.Args)] = ApprovalEntry{Tool: e.Tool, Args: cloneArgs(e.Args)}
	}
}

// GlobalApprovals returns the persisted global approvals, sorted by key for
// stable on-disk output.
func (p *Policy) GlobalApprovals() []ApprovalEntry {
	keys := make([]string, 0, len(p.global))
	for k := range p.global {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ApprovalEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, p.global[k])
	}
	return out
}

// Evaluate applies schema validation, then the documented policy order:
// blacklist (always wins) → global whitelist → session whitelist → approval.
func (p *Policy) Evaluate(d Descriptor, args map[string]string) Verdict {
	if err := d.Validate(args); err != nil {
		return Verdict{Deny, ReasonSchema}
	}
	for _, r := range p.blacklist {
		if r.Tool != "*" && r.Tool != d.ID {
			continue
		}
		if r.Pattern == nil || matchesAny(r.Pattern, args) {
			reason := r.Reason
			if reason == "" {
				reason = ReasonBlacklist
			}
			return Verdict{Deny, reason}
		}
	}
	key := approvalKey(d.ID, args)
	if _, ok := p.global[key]; ok || p.session[key] {
		return Verdict{Allow, ""}
	}
	if d.RequiresApproval {
		return Verdict{NeedsApproval, ReasonApproval}
	}
	return Verdict{Allow, ""}
}

// approvalKey identifies an approval by tool id plus canonical (sorted) args, so
// a tool can be approved for one argument set while a different one is not.
func approvalKey(id string, args map[string]string) string {
	names := make([]string, 0, len(args))
	for n := range args {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(id)
	for _, n := range names {
		b.WriteByte(0)
		b.WriteString(n)
		b.WriteByte('=')
		b.WriteString(args[n])
	}
	return b.String()
}

func cloneArgs(args map[string]string) map[string]string {
	out := make(map[string]string, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}

func matchesAny(re *regexp.Regexp, args map[string]string) bool {
	for _, v := range args {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}
