package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

// RuleSpec is the on-disk form of a blacklist Rule (the compiled regexp is stored
// as its pattern string). See docs/implementation/05.
type RuleSpec struct {
	Tool    string `toml:"tool"`
	Pattern string `toml:"pattern"`
	Reason  string `toml:"reason"`
}

// LoadBlacklist reads blacklist rules from a TOML file:
//
//	[[rule]]
//	tool    = "*"
//	pattern = "/\\.ssh/"
//	reason  = "sensitive_path"
//
// A missing file yields no rules (not an error). Empty patterns deny the tool
// unconditionally.
func LoadBlacklist(path string) ([]Rule, error) {
	if path == "" {
		return nil, nil
	}
	var doc struct {
		Rule []RuleSpec `toml:"rule"`
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load blacklist %s: %w", path, err)
	}
	rules := make([]Rule, 0, len(doc.Rule))
	for _, s := range doc.Rule {
		r := Rule{Tool: s.Tool, Reason: s.Reason}
		if s.Pattern != "" {
			re, err := regexp.Compile(s.Pattern)
			if err != nil {
				return nil, fmt.Errorf("blacklist %s: bad pattern %q: %w", path, s.Pattern, err)
			}
			r.Pattern = re
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// LoadApprovals reads persisted global approvals from a TOML file:
//
//	[[approval]]
//	tool = "read_file"
//	[approval.args]
//	path = "/home/me/notes.txt"
//
// A missing file yields no entries (not an error).
func LoadApprovals(path string) ([]ApprovalEntry, error) {
	if path == "" {
		return nil, nil
	}
	var doc struct {
		Approval []ApprovalEntry `toml:"approval"`
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load approvals %s: %w", path, err)
	}
	return doc.Approval, nil
}

// SaveApprovals writes the global approvals to path (creating parent dirs),
// replacing any previous content.
func SaveApprovals(path string, entries []ApprovalEntry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create approvals dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create approvals %s: %w", path, err)
	}
	defer f.Close()
	doc := struct {
		Approval []ApprovalEntry `toml:"approval"`
	}{Approval: entries}
	if err := toml.NewEncoder(f).Encode(doc); err != nil {
		return fmt.Errorf("write approvals %s: %w", path, err)
	}
	return nil
}
